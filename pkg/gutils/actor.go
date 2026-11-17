// El actor: única goroutine dueña del estado del contrato (goblin → pasos →
// workers). Mutaciones por NDJSON, salidas y muerte del instructor llegan
// por channel. La semántica vive en pkg/contract; acá solo se aplican sus
// efectos sobre cgroupfs (gg) y se ejecutan hooks.
package gutils

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"goblin/pkg/contract"
	"goblin/pkg/manifest"
)

// worker es un proceso lanzado por submit, con su cgroup relativo y el
// *exec.Cmd para poder Wait()/reap.
type worker struct {
	pid  int
	name string // "<id>/step-N/w-I", relativo a la base de gg
	step string
	cmd  *exec.Cmd
}

// cmd es un mensaje interno: un Event de entrada más el channel de respuesta.
type cmd struct {
	event Event
	reply chan Event
}

// actor es el loop dueño del estado. Todo lo que no sea la goroutine del
// actor interactúa por channels.
type actor struct {
	cmds        chan cmd
	stopped     chan struct{}
	cfg         Config
	nextStep    int
	steps       map[string]*contract.Step
	workers     map[int]*worker
	goblinState contract.GoblinState
	orphaned    bool
	createdAt   time.Time
	finish      bool
}

func newActor(cfg Config) *actor {
	return &actor{
		cmds:        make(chan cmd),
		stopped:     make(chan struct{}),
		cfg:         cfg,
		steps:       make(map[string]*contract.Step),
		workers:     make(map[int]*worker),
		goblinState: contract.GoblinAlive,
		createdAt:   time.Now(),
	}
}

// Handle es el Handler que se le pasa a Serve (mutaciones).
func (a *actor) Handle(req Event) Event {
	reply := make(chan Event, 1)
	select {
	case a.cmds <- cmd{event: req, reply: reply}:
		return <-reply
	case <-a.stopped:
		return Event{Type: "error", Error: "goblin: ya terminó"}
	}
}

// notify mete un evento sin esperar respuesta (watchers de salida/pipe/tick).
func (a *actor) notify(evt Event) {
	select {
	case a.cmds <- cmd{event: evt, reply: make(chan Event, 1)}:
	case <-a.stopped:
	}
}

// --- acceso a cgroupfs y hooks ----------------------------------------

func (a *actor) parent() string { return filepath.Dir(a.cfg.CGroupPath) }

func (a *actor) cgroupAbs(name string) string { return filepath.Join(a.parent(), name) }

// gg llama a la herramienta shell con la base apuntando al padre del
// cgroup de este goblin, para que los nombres "<id>/step-N/w-I" resuelvan.
func (a *actor) gg(args ...string) error {
	path := a.cfg.GGPath
	if path == "" {
		path = "gg"
	}
	command := exec.Command(path, args...) // #nosec G204 -- args construidos por el actor
	command.Env = append(os.Environ(), "GOBLIN_CGROUP="+a.parent())
	if out, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("gg %v: %w: %s", args, err, out)
	}
	return nil
}

// readPeak lee memory.peak y el contador oom_kill del worker.
func (a *actor) readPeak(name string) (uint64, bool) {
	base := a.cgroupAbs(name)
	data, _ := os.ReadFile(filepath.Join(base, "memory.peak"))
	peak, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)

	events, _ := os.ReadFile(filepath.Join(base, "memory.events"))
	oom := false
	for _, line := range strings.Split(string(events), "\n") {
		if strings.HasPrefix(line, "oom_kill ") {
			n, _ := strconv.Atoi(strings.Fields(line)[1])
			oom = n > 0
		}
	}
	return peak, oom
}

func (a *actor) runHook(command string) {
	if strings.TrimSpace(command) == "" {
		return
	}
	c := exec.Command("sh", "-c", command) // #nosec G204 -- hook declarado por el instructor
	c.Env = append(os.Environ(), "GOBLIN_ID="+a.cfg.ID, "GOBLIN_STATE="+string(a.goblinState))
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "goblin: hook: %v\n", err)
	}
}

func (a *actor) updateManifest(state contract.GoblinState) {
	a.goblinState = state
	m := &manifest.Manifest{
		ID:        a.cfg.ID,
		PID:       os.Getpid(),
		CGroup:    a.cfg.CGroupPath,
		Socket:    a.cfg.SocketPath,
		OwnerPID:  a.cfg.OwnerPID,
		State:     string(state),
		CreatedAt: a.createdAt,
	}
	if err := manifest.Write(m); err != nil {
		fmt.Fprintf(os.Stderr, "goblin: manifiesto: %v\n", err)
	}
}

// apply ejecuta los efectos devueltos por pkg/contract.
func (a *actor) apply(effects []contract.Effect) {
	for _, e := range effects {
		switch e.Kind {
		case "abort":
			if step := a.steps[e.Step]; step != nil {
				for _, w := range step.Workers {
					if w.State == contract.WorkerRunning || w.State == contract.WorkerSpawning {
						_ = a.gg("kill", w.CGroup)
					}
				}
			}
		case "notify":
			command := e.Command
			if strings.TrimSpace(command) == "" {
				command = a.cfg.NotifyHook
			}
			a.runHook(command)
		case "hook":
			a.runHook(e.Command)
		case "close":
			known := make(map[int]bool, len(a.workers))
			for pid := range a.workers {
				known[pid] = true
			}
			a.cleanupStep(e.Step, known)
			if a.orphaned && a.noActiveSteps() {
				a.finish = true
			}
		}
	}
}

// cleanupStep aplica on_leftover: term a lo que quedó en el cgroup del
// paso, espera, y luego kill + remove. Se hace en goroutine para no
// bloquear el actor mientras espera.
func (a *actor) cleanupStep(stepID string, known map[int]bool) {
	name := a.cfg.ID + "/" + stepID
	seconds := a.cfg.LeftoverSeconds
	if seconds <= 0 {
		seconds = 10
	}
	go func() {
		data, _ := os.ReadFile(filepath.Join(a.cgroupAbs(name), "cgroup.procs"))
		for _, pidStr := range strings.Fields(string(data)) {
			pid, err := strconv.Atoi(pidStr)
			if err == nil && !known[pid] {
				_ = syscall.Kill(pid, syscall.SIGTERM)
			}
		}
		time.Sleep(time.Duration(seconds) * time.Second)
		_ = a.gg("kill", name)
		_ = a.gg("remove", name)
	}()
}

func (a *actor) noActiveSteps() bool {
	for _, s := range a.steps {
		switch s.State {
		case contract.StepPending, contract.StepStarting, contract.StepActive, contract.StepDraining:
			return false
		}
	}
	return true
}

// --- submit ------------------------------------------------------------

func (a *actor) submit(e Event) Event {
	a.nextStep++
	stepID := e.Step
	if stepID == "" {
		stepID = "step-" + strconv.Itoa(a.nextStep)
	}
	if _, exists := a.steps[stepID]; exists {
		return Event{Type: "error", Error: "el paso ya existe: " + stepID}
	}

	name := a.cfg.ID + "/" + stepID
	if err := a.gg("create", name); err != nil {
		return Event{Type: "error", Error: err.Error()}
	}
	if t := e.Terms; t.MemoryMB > 0 || t.CPUCores > 0 || t.Workers > 0 {
		var limits []string
		if t.MemoryMB > 0 {
			limits = append(limits, fmt.Sprintf("memory=%dM", t.MemoryMB))
		}
		if t.CPUCores > 0 {
			limits = append(limits, fmt.Sprintf("cpu=%d", int(t.CPUCores)))
		}
		if t.Workers > 0 {
			limits = append(limits, fmt.Sprintf("pids=%d", t.Workers*2))
		}
		if len(limits) > 0 {
			if err := a.gg(append([]string{"limit", name}, limits...)...); err != nil {
				return Event{Type: "error", Error: err.Error()}
			}
		}
	}

	step := contract.NewStep(stepID, a.cfg.Rules, a.cfg.OnComplete, a.cfg.OnFail)
	step.State = contract.StepStarting
	a.steps[stepID] = step

	n := e.Terms.Workers
	if n < 1 {
		n = 1
	}
	var firstPID int
	for i := 0; i < n; i++ {
		wname := fmt.Sprintf("%s/w-%d", name, i+1)
		if err := a.gg("create", wname); err != nil {
			return Event{Type: "error", Error: err.Error()}
		}
		pr, pw := io.Pipe()
		proc, err := SpawnInto(a.cfg.ID, a.cgroupAbs(wname), e.Cmd, pw, pw)
		if err != nil {
			_ = pw.Close()
			_ = pr.Close()
			_ = a.gg("remove", wname)
			return Event{Type: "error", Error: err.Error()}
		}
		pid := proc.Process.Pid
		if firstPID == 0 {
			firstPID = pid
		}
		a.workers[pid] = &worker{pid: pid, name: wname, step: stepID, cmd: proc}
		step.AddWorker(pid, wname)

		// Salida: el actor la registra para las reglas de texto.
		go func(pid int, reader *io.PipeReader) {
			defer reader.Close()
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				a.notify(Event{Type: "output", PID: pid, Error: scanner.Text()})
			}
		}(pid, pr)

		// Espera: Wait() hace reap y el evento mueve el estado del worker.
		go func(pid int, wname string, proc *exec.Cmd, writer *io.PipeWriter) {
			err := proc.Wait()
			_ = writer.Close()
			code := 0
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
			_, oom := a.readPeak(wname)
			a.notify(Event{Type: "worker_exit", PID: pid, ExitCode: code, OOM: oom})
		}(pid, wname, proc, pw)
	}
	step.State = contract.StepActive
	a.updateManifest(contract.GoblinRunning)
	return Event{Type: "submit", PID: firstPID, Step: stepID, State: StateRunning}
}

// --- loop principal ----------------------------------------------------

func (a *actor) run() {
	defer close(a.stopped)
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			a.notify(Event{Type: "tick"})
		}
	}()

	for c := range a.cmds {
		var resp Event
		e := c.event

		switch e.Type {
		case "submit":
			resp = a.submit(e)

		case "worker_exit":
			if w := a.workers[e.PID]; w != nil {
				if step := a.steps[w.step]; step != nil {
					peak, oom := a.readPeak(w.name)
					a.apply(step.OnWorkerExit(e.PID, e.ExitCode, e.OOM || oom, time.Now(), peak, oom))
					if step.AllWorkersDone() {
						a.apply(step.Close(time.Now(), peak, oom))
					}
				}
				delete(a.workers, e.PID)
			}
			resp = Event{Type: "ok", PID: e.PID}

		case "output":
			if w := a.workers[e.PID]; w != nil {
				if step := a.steps[w.step]; step != nil {
					step.OnOutput(e.Error + "\n")
					peak, oom := a.readPeak(w.name)
					a.apply(step.Tick(time.Now(), peak, oom))
				}
			}
			resp = Event{Type: "ok"}

		case "tick":
			for _, step := range a.steps {
				if step.State != contract.StepActive {
					continue
				}
				a.apply(step.Tick(time.Now(), 0, false))
			}
			resp = Event{Type: "ok"}

		case "pause", "resume", "kill":
			w := a.workers[e.PID]
			if w == nil {
				resp = Event{Type: "error", Error: "pid desconocido"}
				break
			}
			verb, newState := "", StateRunning
			switch e.Type {
			case "pause":
				verb, newState = "freeze", StatePaused
			case "resume":
				verb, newState = "thaw", StateRunning
			case "kill":
				verb, newState = "kill", StateKilled
			}
			if err := a.gg(verb, w.name); err != nil {
				resp = Event{Type: "error", PID: e.PID, Error: err.Error()}
				break
			}
			resp = Event{Type: "ok", PID: e.PID, State: newState}

		case "list":
			var lines []string
			for _, s := range a.steps {
				lines = append(lines, s.Status())
			}
			resp = Event{Type: "list", Error: strings.Join(lines, " ")}

		case "death":
			resp = a.onDeath()

		default:
			resp = Event{Type: "error", Error: "verbo desconocido: " + e.Type}
		}

		c.reply <- resp
		if a.finish {
			return
		}
	}
}

// onDeath aplica on_orphan. La garantía dura (goblin muerto → workers
// muertos) la da cgroupfs: si este proceso muere de golpe, los workers
// siguen en su cgroup pero Pdeathsig y gg kill al cierre cubren el caso.
func (a *actor) onDeath() Event {
	policy := a.cfg.OrphanPolicy
	if policy == "" {
		policy = "keep"
	}
	switch policy {
	case "kill":
		for _, s := range a.steps {
			for _, w := range s.Workers {
				_ = a.gg("kill", w.CGroup)
			}
		}
		a.updateManifest(contract.GoblinDying)
		a.finish = true
	case "freeze":
		for _, s := range a.steps {
			for _, w := range s.Workers {
				_ = a.gg("freeze", w.CGroup)
			}
		}
		a.updateManifest(contract.GoblinOrphaned)
		a.finish = true
	default: // keep: sobrevivimos al instructor con los pasos que queden
		a.orphaned = true
		a.updateManifest(contract.GoblinOrphaned)
		if a.noActiveSteps() {
			a.finish = true
		}
	}
	a.runHook(a.cfg.NotifyHook)
	return Event{Type: "death", Error: "on_orphan=" + policy}
}

// watchDeathPipe ve EOF cuando muere el instructor, incluso por SIGKILL.
func watchDeathPipe(deathPipe *os.File, a *actor) {
	buf := make([]byte, 1)
	for {
		if _, err := deathPipe.Read(buf); err != nil {
			a.notify(Event{Type: "death"})
			return
		}
	}
}

// Run levanta el goblin efímero: manifiesto, actor, watcher, servidor.
func Run(cfg Config) error {
	a := newActor(cfg)
	go a.run()

	if cfg.DeathPipeFD >= 0 {
		deathPipe := os.NewFile(uintptr(cfg.DeathPipeFD), "deathpipe")
		go watchDeathPipe(deathPipe, a)
	}

	a.updateManifest(contract.GoblinAlive)

	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(cfg.SocketPath, a.Handle) }()

	select {
	case err := <-serveErr:
		_ = os.Remove(cfg.SocketPath)
		_ = manifest.Remove(cfg.ID)
		return err
	case <-a.stopped:
		a.updateManifest(contract.GoblinDead)
		_ = os.Remove(cfg.SocketPath)
		_ = manifest.Remove(cfg.ID)
		return nil
	}
}

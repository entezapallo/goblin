// El actor: única goroutine dueña del estado del contrato (goblin → pasos →
// workers). Mutaciones por NDJSON, salidas y muerte del instructor llegan
// por channel. La semántica vive en pkg/contract; acá solo se aplican sus
// efectos sobre cgroupfs (gg) y se ejecutan hooks.
package gutils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"goblin/pkg/applets/gprocutil"
	"goblin/pkg/contract"
	"goblin/pkg/history"
	"goblin/pkg/manifest"
	"goblin/pkg/notify"
)

// worker es un proceso lanzado por submit, con su cgroup relativo y el
// *exec.Cmd para poder Wait()/reap.
type worker struct {
	pid      int
	name     string // "<id>/step-N/w-I", relativo a la base de gg
	step     string
	cmd      *exec.Cmd
	workDir  string   // directorio normal (no cgroupfs) para status.json
	liveness *os.File // write end del pipe que sostiene el guard
}

// cmd es un mensaje interno: un Event de entrada más el channel de respuesta.
type cmd struct {
	event Event
	reply chan Event
}

// actor es el loop dueño del estado. Todo lo que no sea la goroutine del
// actor interactúa por channels.
type actor struct {
	cmds         chan cmd
	stopped      chan struct{}
	cfg          Config
	nextStep     int
	steps        map[string]*contract.Step
	workers      map[int]*worker
	treeSamplers map[string]*TreeSampler
	treeStops    map[string]chan struct{}
	goblinState  contract.GoblinState
	orphaned     bool
	createdAt    time.Time
	finish       bool
}

func newActor(cfg Config) *actor {
	return &actor{
		cmds:         make(chan cmd),
		stopped:      make(chan struct{}),
		cfg:          cfg,
		steps:        make(map[string]*contract.Step),
		workers:      make(map[int]*worker),
		treeSamplers: make(map[string]*TreeSampler),
		treeStops:    make(map[string]chan struct{}),
		goblinState:  contract.GoblinAlive,
		createdAt:    time.Now(),
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

// runtimeDir es donde viven manifiestos, notificaciones, historial y workdirs.
// firstChannel extrae el primer canal de un header `notify email=x sms=y`.
func firstChannel(header string) string {
	fields := strings.Fields(header)
	if len(fields) == 0 {
		return "default"
	}
	if i := strings.IndexByte(fields[0], '='); i > 0 {
		return fields[0][:i]
	}
	return fields[0]
}

func (a *actor) runtimeDir() string {
	if a.cfg.RuntimeDir != "" {
		return a.cfg.RuntimeDir
	}
	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		return v
	}
	return os.TempDir()
}

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

func (a *actor) runHook(command string, labels map[string]string) {
	if strings.TrimSpace(command) == "" {
		return
	}
	c := exec.Command("sh", "-c", command) // #nosec G204 -- hook declarado por el instructor
	env := append(os.Environ(), "GOBLIN_ID="+a.cfg.ID, "GOBLIN_STATE="+string(a.goblinState))
	for k, v := range labels {
		env = append(env, "GOBLIN_LABEL_"+k+"="+v)
	}
	c.Env = env
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "goblin: hook: %v\n", err)
	}
}

// emit publica un notify_when en notifications.jsonl. `data=log` usa el
// stderr_tail del paso.
func (a *actor) emit(e contract.Effect) {
	channel := e.Channel
	if channel == "" {
		channel = a.cfg.NotifyHook
	}
	if channel == "" {
		channel = "default"
	}
	data := e.Data
	if data == "" {
		data = e.Message
	}
	if data == "log" {
		if step := a.steps[e.Step]; step != nil {
			data = strings.TrimSpace(step.Tail())
		}
	}
	if err := notify.Emit(a.runtimeDir(), channel, data); err != nil {
		fmt.Fprintf(os.Stderr, "goblin: notify: %v\n", err)
	}
}

// recordTree cierra el muestreo del paso y guarda el step_tree en el
// historial del goblin, que es lo que lee orb.
func (a *actor) recordTree(stepID string) {
	if stop := a.treeStops[stepID]; stop != nil {
		close(stop)
		delete(a.treeStops, stepID)
	}
	sampler := a.treeSamplers[stepID]
	if sampler == nil {
		return
	}
	delete(a.treeSamplers, stepID)
	sampler.Sample()
	_ = history.AppendGoblinLogByID(a.cfg.ID, sampler.Snapshot())
}

// recordHistory deja constancia del paso cerrado y del worker que falló.
func (a *actor) recordHistory(step *contract.Step) {
	if step == nil {
		return
	}
	var workerPID int
	if len(step.Workers) > 0 {
		workerPID = step.Workers[0].PID
	}
	_ = history.AppendSummary(history.SummaryEntry{
		GoblinPID: os.Getpid(),
		WorkerPID: workerPID,
		Cmd:       step.Cmd,
		State:     string(step.Outcome),
		StartedAt: step.StartedAt,
		EndedAt:   step.ClosedAt,
	})
	_ = history.AppendGoblinLog(os.Getpid(), map[string]any{
		"event": "step_closed", "step": step.ID,
		"state": step.Status(), "labels": step.Labels,
	})
}

// recordFailure construye el struct de depuración del worker fallido.
func (a *actor) recordFailure(step *contract.Step, w *worker, code int, oom bool) {
	if step == nil || w == nil || (code == 0 && !oom) {
		return
	}
	tail := strings.Split(strings.TrimRight(step.Tail(), "\n"), "\n")
	f := contract.Failure{
		Worker:     w.name,
		Labels:     step.Labels,
		Exit:       contract.FailureExit{Code: code, OOM: oom},
		StderrTail: tail,
	}
	if code == -1 {
		signal := "killed"
		f.Exit.Signal = &signal
	}
	if data, err := os.ReadFile(filepath.Join(w.workDir, "status.json")); err == nil {
		var status map[string]any
		if json.Unmarshal(data, &status) == nil {
			f.Status = status
		}
	}
	dir := filepath.Join(a.runtimeDir(), "goblin")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(dir, a.cfg.ID+".failures.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // #nosec G304 -- path derivado del runtime dir
	if err != nil {
		return
	}
	defer file.Close()
	_ = json.NewEncoder(file).Encode(f)
}

func (a *actor) updateManifest(state contract.GoblinState) {
	a.goblinState = state
	_, _, _, _, _, _, _, _, selfStart := gprocutil.ReadStat(os.Getpid())
	_, _, _, _, _, _, _, _, ownerStart := gprocutil.ReadStat(a.cfg.OwnerPID)
	m := &manifest.Manifest{
		ID:             a.cfg.ID,
		PID:            os.Getpid(),
		Starttime:      selfStart,
		CGroup:         a.cfg.CGroupPath,
		Socket:         a.cfg.SocketPath,
		OwnerPID:       a.cfg.OwnerPID,
		OwnerStarttime: ownerStart,
		State:          string(state),
		CreatedAt:      a.createdAt,
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
			a.emit(e)
		case "hook":
			var labels map[string]string
			if step := a.steps[e.Step]; step != nil {
				labels = step.Labels
			}
			a.runHook(e.Command, labels)
		case "close":
			a.recordTree(e.Step)
			a.recordHistory(a.steps[e.Step])
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
			limits = append(limits, fmt.Sprintf("cpu=%g", t.CPUCores))
		}
		if t.Workers > 0 {
			// pids.max cuenta threads en cgroup v2; un binario Go necesita
			// varias decenas. El techo es por job, no por hilo del runtime.
			n := t.Workers*64 + 64
			if n < 256 {
				n = 256
			}
			limits = append(limits, fmt.Sprintf("pids=%d", n))
		}
		if len(limits) > 0 {
			if err := a.gg(append([]string{"limit", name}, limits...)...); err != nil {
				return Event{Type: "error", Error: err.Error()}
			}
		}
	}

	rules := a.cfg.Rules
	if len(e.Rules) > 0 {
		compiled, err := contract.CompileAll(e.Rules)
		if err != nil {
			return Event{Type: "error", Error: err.Error()}
		}
		rules = compiled
	}
	onComplete, onFail := a.cfg.OnComplete, a.cfg.OnFail
	if e.OnComplete != "" {
		onComplete = e.OnComplete
	}
	if e.OnFail != "" {
		onFail = e.OnFail
	}
	step := contract.NewStep(stepID, rules, onComplete, onFail, e.Labels, e.Cmd)
	step.SetFileSize(func(path string) int64 {
		if path == "" {
			return 0
		}
		full := filepath.Join(a.runtimeDir(), "goblin", a.cfg.ID, path)
		info, err := os.Stat(full) // #nosec G304 -- path viene de la DSL del instructor
		if err != nil {
			return 0
		}
		return info.Size()
	})
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
		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()
		// Pipe de liveness: el goblin guarda el write end; el guard del worker
		// recibe el read end por fd 3. Si el goblin muere, el kernel cierra
		// el write end y el guard mata al worker sin código nuestro vivo.
		workDir := filepath.Join(a.runtimeDir(), a.cfg.ID, stepID, "w-"+strconv.Itoa(i+1))
		if err := os.MkdirAll(workDir, 0700); err != nil {
			return Event{Type: "error", Error: err.Error()}
		}
		liveRead, liveWrite, err := os.Pipe()
		if err != nil {
			_ = stdoutW.Close()
			_ = stdoutR.Close()
			_ = stderrW.Close()
			_ = stderrR.Close()
			return Event{Type: "error", Error: err.Error()}
		}
		proc, err := SpawnInto(a.cfg.ID, a.cgroupAbs(wname), workDir, e.Cmd, stdoutW, stderrW, liveRead, a.cfg.GuardBin)
		_ = liveRead.Close() // el hijo ya tiene su copia
		if err != nil {
			_ = liveWrite.Close()
			_ = stdoutW.Close()
			_ = stdoutR.Close()
			_ = stderrW.Close()
			_ = stderrR.Close()
			_ = a.gg("remove", wname)
			return Event{Type: "error", Error: err.Error()}
		}
		pid := proc.Process.Pid
		if firstPID == 0 {
			firstPID = pid
		}
		a.workers[pid] = &worker{pid: pid, name: wname, step: stepID, cmd: proc, workDir: workDir, liveness: liveWrite}
		step.AddWorker(pid, wname)

		// Salida separada por flujo para reglas `match ... in stdout|stderr`.
		for stream, reader := range map[string]*io.PipeReader{"stdout": stdoutR, "stderr": stderrR} {
			go func(stream string, reader *io.PipeReader) {
				defer reader.Close()
				scanner := bufio.NewScanner(reader)
				for scanner.Scan() {
					a.notify(Event{Type: "output", PID: pid, Stream: stream, Line: scanner.Text()})
				}
			}(stream, reader)
		}

		// Espera: Wait() hace reap y el evento mueve el estado del worker.
		go func(pid int, wname string, proc *exec.Cmd, outW, errW *io.PipeWriter) {
			err := proc.Wait()
			_ = outW.Close()
			_ = errW.Close()
			code := 0
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
			_, oom := a.readPeak(wname)
			a.notify(Event{Type: "worker_exit", PID: pid, ExitCode: code, OOM: oom})
		}(pid, wname, proc, stdoutW, stderrW)
	}
	// Muestreo del árbol por cambio de populated, con resguardo de 5 min.
	sampler := NewTreeSampler("", a.cgroupAbs(name), stepID)
	stopTree := make(chan struct{})
	a.treeSamplers[stepID] = sampler
	a.treeStops[stepID] = stopTree
	go func() {
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stopTree:
				return
			case <-tick.C:
				if sampler.ShouldSample(time.Now()) {
					sampler.Sample()
				}
			}
		}
	}()

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
					step.AddMemSample(peak)
					if oom || e.OOM {
						step.AddOOM()
					} else if e.ExitCode == -1 {
						step.AddSevere()
					}
					a.recordFailure(step, w, e.ExitCode, e.OOM || oom)
					a.apply(step.OnWorkerExit(e.PID, e.ExitCode, e.OOM || oom, time.Now(), peak, oom))
					if step.AllWorkersDone() {
						a.apply(step.Close(time.Now(), peak, oom))
					}
				}
				if w.liveness != nil {
					_ = w.liveness.Close()
				}
				delete(a.workers, e.PID)
			}
			resp = Event{Type: "ok", PID: e.PID}

		case "output":
			if w := a.workers[e.PID]; w != nil {
				if step := a.steps[w.step]; step != nil {
					step.OnOutputStream(e.Stream, e.Line)
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
			steps := make([]StepInfo, 0, len(a.steps))
			for _, s := range a.steps {
				steps = append(steps, StepInfo{
					ID: s.ID, State: string(s.State),
					Outcome: string(s.Outcome), Workers: len(s.Workers),
				})
			}
			resp = Event{Type: "list", Steps: steps}

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
// muertos) no depende de este código: cada worker tiene un gguard con un
// pipe cuyo write end solo sostiene este proceso. Si muere de golpe, el
// kernel cierra el pipe y los guards matan a sus hijos. Acá solo se cierra
// explícitamente cuando la política es kill.
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
		for _, w := range a.workers {
			if w.liveness != nil {
				_ = w.liveness.Close()
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
	// El header notify se publica como canal por defecto; sin regla
	// disparada, esto deja constancia de la muerte del instructor.
	_ = notify.Emit(a.runtimeDir(), firstChannel(a.cfg.NotifyHook), "on_orphan="+policy)
	return Event{Type: "death", Message: "on_orphan=" + policy}
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

	var connWG sync.WaitGroup
	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(cfg.SocketPath, a.Handle, &connWG) }()

	select {
	case err := <-serveErr:
		connWG.Wait()
		_ = os.Remove(cfg.SocketPath)
		_ = manifest.Remove(cfg.ID)
		return err
	case <-a.stopped:
		// El actor ya contestó el último evento; esperamos a que esa
		// respuesta llegue al socket antes de que el proceso muera.
		connWG.Wait()
		a.updateManifest(contract.GoblinDead)
		_ = os.Remove(cfg.SocketPath)
		_ = manifest.Remove(cfg.ID)
		return nil
	}
}

// Package contract es la semántica del contract: estados, hechos, reglas
// y hooks. Es lógica pura — no toca cgroupfs ni lanza procesos. Devuelve
// Effects que el actor de gutils aplica (gg kill, sh -c, etc.).
package contract

import (
	"bytes"
	"fmt"
	"regexp"
	"time"
)

// GoblinState: unborn → alive → running → dying → dead. orphaned es el
// estado cuando murió el instructor y on_orphan=keep.
type GoblinState string

const (
	GoblinAlive    GoblinState = "alive"
	GoblinRunning  GoblinState = "running"
	GoblinOrphaned GoblinState = "orphaned"
	GoblinDying    GoblinState = "dying"
	GoblinDead     GoblinState = "dead"
)

// StepState y WorkerState reflejan la jerarquía goblin → paso → worker.
type StepState string

const (
	StepPending  StepState = "pending"
	StepStarting StepState = "starting"
	StepActive   StepState = "active"
	StepDraining StepState = "draining"
	StepClosing  StepState = "closing"
	StepClosed   StepState = "closed"
)

type WorkerState string

const (
	WorkerSpawning  WorkerState = "spawning"
	WorkerRunning   WorkerState = "running"
	WorkerExited    WorkerState = "exited"
	WorkerKilled    WorkerState = "killed"
	WorkerOOMKilled WorkerState = "oom_killed"
)

type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailed  Outcome = "failed"
	OutcomeAborted Outcome = "aborted"
)

// Facts es lo medible en un instante. El motor de reglas solo ve esto.
type Facts struct {
	OOM          bool
	MemOver      bool
	MemPeakBytes uint64
	Elapsed      time.Duration
	FailedCount  int
	WorkerCount  int
	Text         string // tail de stdout+stderr del paso
}

// RuleKind: fail_when marca el paso fallido al cerrar; abort_when es
// fail-fast y mata a los demás; notify_when solo notifica (con enfriamiento).
type RuleKind string

const (
	RuleFail   RuleKind = "fail_when"
	RuleAbort  RuleKind = "abort_when"
	RuleNotify RuleKind = "notify_when"
)

// Rule es un predicado sobre Facts más, para notify, un comando a ejecutar.
type Rule struct {
	Kind    RuleKind
	Name    string
	When    func(Facts) bool
	Command string // hook de notificación
}

func textRule(kind RuleKind, name, pattern string) Rule {
	re, err := regexp.Compile(pattern)
	return Rule{Kind: kind, Name: name, When: func(f Facts) bool {
		if err != nil {
			return false
		}
		return re.MatchString(f.Text)
	}}
}

// TextRule dispara cuando el patrónregex aparece en la salida del paso.
func TextRule(kind RuleKind, name, pattern string) Rule { return textRule(kind, name, pattern) }

// OOMRule dispara cuando algún worker fue matado por el kernel por memoria.
func OOMRule(kind RuleKind, name string) Rule {
	return Rule{Kind: kind, Name: name, When: func(f Facts) bool { return f.OOM }}
}

// FailedCountRule dispara cuando fallaron al menos n workers.
func FailedCountRule(kind RuleKind, name string, n int) Rule {
	return Rule{Kind: kind, Name: name, When: func(f Facts) bool { return f.FailedCount >= n }}
}

// TimeoutRule dispara cuando pasó d desde el inicio del paso.
func TimeoutRule(kind RuleKind, name string, d time.Duration) Rule {
	return Rule{Kind: kind, Name: name, When: func(f Facts) bool { return f.Elapsed >= d }}
}

// MemOverRule dispara cuando el pico de memoria supera bytes.
func MemOverRule(kind RuleKind, name string, bytes uint64) Rule {
	return Rule{Kind: kind, Name: name, When: func(f Facts) bool { return f.MemPeakBytes >= bytes }}
}

// Effect es una acción que el actor debe ejecutar fuera del paquete.
type Effect struct {
	Kind    string // "abort", "notify", "hook", "close"
	Step    string
	Command string
	Outcome Outcome
	Message string
}

const tailLimit = 50 * 1024 // últimos 50 KB del stderr, como fija el diseño

// Worker es un proceso del paso.
type Worker struct {
	PID      int
	CGroup   string
	State    WorkerState
	ExitCode int
	OOM      bool
}

// Step agrupa workers, reglas y hooks, y guarda su propio ciclo de vida.
type Step struct {
	ID         string
	State      StepState
	Outcome    Outcome
	Workers    []*Worker
	Rules      []Rule
	OnComplete string
	OnFail     string
	StartedAt  time.Time

	tail       bytes.Buffer
	aborted    bool
	notifiedAt map[string]time.Time
	cooldown   time.Duration
}

// NewStep crea un paso en pending.
func NewStep(id string, rules []Rule, onComplete, onFail string) *Step {
	return &Step{
		ID:         id,
		State:      StepPending,
		Rules:      rules,
		OnComplete: onComplete,
		OnFail:     onFail,
		StartedAt:  time.Now(),
		notifiedAt: make(map[string]time.Time),
		cooldown:   time.Minute,
	}
}

// AddWorker registra un worker y pasa el paso a active si estaba starting.
func (s *Step) AddWorker(pid int, cgroup string) {
	s.Workers = append(s.Workers, &Worker{PID: pid, CGroup: cgroup, State: WorkerRunning})
	if s.State == StepPending || s.State == StepStarting {
		s.State = StepActive
	}
}

// Worker devuelve el worker por pid.
func (s *Step) Worker(pid int) *Worker {
	for _, w := range s.Workers {
		if w.PID == pid {
			return w
		}
	}
	return nil
}

// OnOutput guarda la salida (con tope) para que las reglas de texto la vean.
func (s *Step) OnOutput(line string) {
	s.tail.WriteString(line)
	if s.tail.Len() > tailLimit {
		rest := s.tail.Bytes()[s.tail.Len()-tailLimit:]
		s.tail.Reset()
		s.tail.Write(rest)
	}
}

// Facts calcula los hechos actuales del paso.
func (s *Step) Facts(now time.Time, peakBytes uint64, memOver bool) Facts {
	f := Facts{
		MemPeakBytes: peakBytes,
		MemOver:      memOver,
		Elapsed:      now.Sub(s.StartedAt),
		WorkerCount:  len(s.Workers),
		Text:         s.tail.String(),
	}
	for _, w := range s.Workers {
		if w.OOM {
			f.OOM = true
		}
		if w.State == WorkerExited && w.ExitCode != 0 ||
			w.State == WorkerKilled || w.State == WorkerOOMKilled {
			f.FailedCount++
		}
	}
	return f
}

// OnWorkerExit registra el final de un worker y evalúa abort_when /
// notify_when en caliente. Devuelve los efectos a aplicar.
func (s *Step) OnWorkerExit(pid int, code int, oom bool, now time.Time, peakBytes uint64, memOver bool) []Effect {
	w := s.Worker(pid)
	if w == nil {
		return nil
	}
	w.ExitCode = code
	w.OOM = oom
	switch {
	case oom:
		w.State = WorkerOOMKilled
	case code != 0:
		w.State = WorkerExited
	default:
		w.State = WorkerExited
	}
	return s.evaluate(now, peakBytes, memOver)
}

// Tick reevalúa las reglas con el hecho t (tiempo transcurrido).
func (s *Step) Tick(now time.Time, peakBytes uint64, memOver bool) []Effect {
	if s.State != StepActive {
		return nil
	}
	return s.evaluate(now, peakBytes, memOver)
}

func (s *Step) evaluate(now time.Time, peakBytes uint64, memOver bool) []Effect {
	f := s.Facts(now, peakBytes, memOver)
	var out []Effect
	for _, r := range s.Rules {
		if !r.When(f) {
			continue
		}
		switch r.Kind {
		case RuleAbort:
			if !s.aborted {
				s.aborted = true
				s.State = StepDraining
				out = append(out, Effect{Kind: "abort", Step: s.ID, Message: r.Name})
			}
		case RuleNotify:
			last := s.notifiedAt[r.Name]
			if last.IsZero() || now.Sub(last) >= s.cooldown {
				s.notifiedAt[r.Name] = now
				out = append(out, Effect{Kind: "notify", Step: s.ID, Command: r.Command, Message: r.Name})
			}
		}
	}
	return out
}

// AllWorkersDone indica si el paso puede cerrarse.
func (s *Step) AllWorkersDone() bool {
	if len(s.Workers) == 0 {
		return false
	}
	for _, w := range s.Workers {
		switch w.State {
		case WorkerSpawning, WorkerRunning:
			return false
		}
	}
	return true
}

// Close evalúa fail_when una sola vez, fija el outcome y devuelve el hook
// correspondiente. El abort_when ya puesto el estado en draining.
func (s *Step) Close(now time.Time, peakBytes uint64, memOver bool) []Effect {
	if s.State == StepClosed || s.State == StepClosing {
		return nil
	}
	s.State = StepClosing
	f := s.Facts(now, peakBytes, memOver)
	outcome := OutcomeSuccess
	if s.aborted {
		outcome = OutcomeAborted
	} else if f.FailedCount > 0 {
		outcome = OutcomeFailed
	}
	for _, r := range s.Rules {
		if r.Kind == RuleFail && r.When(f) {
			outcome = OutcomeFailed
			break
		}
	}
	s.Outcome = outcome
	s.State = StepClosed

	hook := s.OnComplete
	if outcome != OutcomeSuccess {
		hook = s.OnFail
	}
	out := []Effect{{Kind: "close", Step: s.ID, Outcome: outcome}}
	if hook != "" {
		out = append(out, Effect{Kind: "hook", Step: s.ID, Command: hook, Outcome: outcome})
	}
	return out
}

// Status es una línea resumen para manifest / list.
func (s *Step) Status() string {
	return fmt.Sprintf("%s:%s:%s:%d", s.ID, s.State, s.Outcome, len(s.Workers))
}

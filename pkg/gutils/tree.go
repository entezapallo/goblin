// pkg/gutils/tree.go
//
// Muestreo del árbol de procesos de un paso. Disparo por cambio de
// cgroup.events:populated, con resguardo de 5 min si el paso está idle.
// La identidad es (pid, starttime) porque el PID se recicla.
package gutils

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxTreeProcesses = 500
	treeIdleFallback = 5 * time.Minute
	argv0MaxLen      = 64
)

// procKey identifica un proceso sin ambigüedad de reciclaje de PID.
type procKey struct {
	pid       int
	starttime uint64
}

// ProcNode es un proceso visto en un muestreo del árbol.
type ProcNode struct {
	PID       int       `json:"pid"`
	Starttime uint64    `json:"starttime"`
	PPID      int       `json:"ppid"`
	Argv0     string    `json:"argv0"`
	Worker    string    `json:"worker"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// TreeSnapshot es el registro step_tree que se guarda en el historial.
type TreeSnapshot struct {
	Event     string     `json:"event"`
	Step      string     `json:"step"`
	Processes []ProcNode `json:"processes"`
	Truncated int        `json:"truncated"`
	SampledAt time.Time  `json:"sampled_at"`
}

// TreeSampler guarda el árbol visto de un paso. procRoot es inyectable para
// tests; en producción es "/proc".
type TreeSampler struct {
	procRoot   string
	stepCgroup string
	stepName   string

	mu        sync.Mutex
	nodes     map[procKey]*ProcNode
	truncated int

	lastPopulated string
	lastSample    time.Time
}

// NewTreeSampler crea un sampler para el cgroup de un paso.
func NewTreeSampler(procRoot, stepCgroup, stepName string) *TreeSampler {
	if procRoot == "" {
		procRoot = "/proc"
	}
	return &TreeSampler{
		procRoot: procRoot, stepCgroup: stepCgroup, stepName: stepName,
		nodes: make(map[procKey]*ProcNode),
	}
}

// Sample registra un muestreo del árbol.
func (t *TreeSampler) Sample() {
	procs := t.collectProcs()
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()
	for _, p := range procs {
		if len(t.nodes) >= maxTreeProcesses && t.nodes[p.key] == nil {
			t.truncated++
			continue
		}
		node := t.nodes[p.key]
		if node == nil {
			t.nodes[p.key] = &ProcNode{
				PID: p.key.pid, Starttime: p.key.starttime, PPID: p.ppid,
				Argv0: p.argv0, Worker: p.worker,
				FirstSeen: now, LastSeen: now,
			}
		} else {
			node.LastSeen = now
		}
	}
}

// Snapshot devuelve una copia del estado actual para el historial.
func (t *TreeSampler) Snapshot() TreeSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	nodes := make([]ProcNode, 0, len(t.nodes))
	for _, n := range t.nodes {
		nodes = append(nodes, *n)
	}
	return TreeSnapshot{
		Event: "step_tree", Step: t.stepName, Processes: nodes,
		Truncated: t.truncated, SampledAt: time.Now(),
	}
}

// ShouldSample decide si hay que muestrear: cambió populated o pasaron
// 5 minutos sin eventos (resguardo para pasos idle).
func (t *TreeSampler) ShouldSample(now time.Time) bool {
	data, err := os.ReadFile(filepath.Join(t.stepCgroup, "cgroup.events")) // #nosec G304
	if err != nil {
		return false
	}
	populated := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "populated ") {
			populated = strings.TrimSpace(strings.TrimPrefix(line, "populated"))
			break
		}
	}
	changed := populated != "" && populated != t.lastPopulated
	idle := !t.lastSample.IsZero() && now.Sub(t.lastSample) >= treeIdleFallback
	if changed || idle || t.lastSample.IsZero() {
		t.lastPopulated = populated
		t.lastSample = now
		return true
	}
	return false
}

type sampledProc struct {
	key    procKey
	ppid   int
	argv0  string
	worker string
}

// collectProcs recorre cgroup.procs de todos los workers del paso.
func (t *TreeSampler) collectProcs() []sampledProc {
	workers, _ := filepath.Glob(filepath.Join(t.stepCgroup, "w-*"))
	var out []sampledProc
	for _, w := range workers {
		workerName := filepath.Base(w)
		data, err := os.ReadFile(filepath.Join(w, "cgroup.procs")) // #nosec G304
		if err != nil {
			continue
		}
		for _, s := range strings.Fields(string(data)) {
			pid, err := strconv.Atoi(s)
			if err != nil {
				continue
			}
			ppid, starttime := t.readStat(pid)
			out = append(out, sampledProc{
				key:  procKey{pid: pid, starttime: starttime},
				ppid: ppid, argv0: t.readArgv0(pid), worker: workerName,
			})
		}
	}
	return out
}

// readStat lee ppid y starttime desde el procRoot inyectado (o /proc).
func (t *TreeSampler) readStat(pid int) (ppid int, starttime uint64) {
	data, err := os.ReadFile(filepath.Join(t.procRoot, strconv.Itoa(pid), "stat")) // #nosec G304
	if err != nil {
		return 0, 0
	}
	s := strings.TrimSpace(string(data))
	idx := strings.LastIndex(s, ") ")
	if idx < 0 {
		return 0, 0
	}
	fields := strings.Fields(s[idx+2:])
	if len(fields) < 22 {
		return 0, 0
	}
	ppid, _ = strconv.Atoi(fields[1])
	starttime, _ = strconv.ParseUint(fields[19], 10, 64)
	return ppid, starttime
}

func (t *TreeSampler) readArgv0(pid int) string {
	data, err := os.ReadFile(filepath.Join(t.procRoot, strconv.Itoa(pid), "cmdline")) // #nosec G304
	if err != nil {
		return ""
	}
	arg := string(data)
	if i := strings.IndexByte(arg, 0); i >= 0 {
		arg = arg[:i]
	}
	if len(arg) > argv0MaxLen {
		arg = arg[:argv0MaxLen]
	}
	return arg
}

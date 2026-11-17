// Package orb reúne el estado que muestra la TUI: goblins vivos (manifiestos)
// y sus workers (cgroupfs). Es solo consulta; las mutaciones van por verbs.
package orb

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"goblin/pkg/discover"
)

// Worker es un proceso de cómputo dentro de un goblin.
type Worker struct {
	PID        int
	Name       string // step-1/w-1
	CGroup     string
	Frozen     bool
	MemCurrent uint64
}

// Goblin es la vista rica que muestra orb: manifiesto + métricas de cgroup.
type Goblin struct {
	ID      string
	PID     int
	State   string
	CGroup  string
	Socket  string
	MemMax  string
	CPUMax  string
	PIDsMax string
	Workers []Worker
}

// List construye el snapshot de la TUI.
func List() ([]Goblin, error) {
	found, err := discover.Discover()
	if err != nil {
		return nil, err
	}
	out := make([]Goblin, 0, len(found))
	for _, g := range found {
		item := Goblin{
			ID: g.ID, PID: g.PID, State: g.State,
			CGroup: g.CGroup, Socket: g.SocketPath,
			MemMax:  readFile(g.CGroup, "memory.max"),
			CPUMax:  readFile(g.CGroup, "cpu.max"),
			PIDsMax: readFile(g.CGroup, "pids.max"),
		}
		item.Workers = listWorkers(g.CGroup)
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// listWorkers recorre step-N/w-I leyendo cgroup.procs y cgroup.events.
func listWorkers(cgroup string) []Worker {
	var out []Worker
	steps, _ := filepath.Glob(filepath.Join(cgroup, "step-*"))
	sort.Strings(steps)
	for _, step := range steps {
		ws, _ := filepath.Glob(filepath.Join(step, "w-*"))
		sort.Strings(ws)
		for _, w := range ws {
			data, err := os.ReadFile(filepath.Join(w, "cgroup.procs")) // #nosec G304
			if err != nil {
				continue
			}
			frozen := strings.Contains(readFile(w, "cgroup.events"), "frozen 1")
			mem, _ := strconv.ParseUint(strings.TrimSpace(readFile(w, "memory.current")), 10, 64)
			for _, pidStr := range strings.Fields(string(data)) {
				pid, err := strconv.Atoi(pidStr)
				if err != nil {
					continue
				}
				out = append(out, Worker{
					PID: pid, Name: filepath.Base(step) + "/" + filepath.Base(w),
					CGroup: w, Frozen: frozen, MemCurrent: mem,
				})
			}
		}
	}
	return out
}

func readFile(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304
	if err != nil {
		return "—"
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "—"
	}
	return s
}

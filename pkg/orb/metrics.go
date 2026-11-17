package orb

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const sparkChars = "▁▂▃▄▅▆▇█"

// Sample es un par de valores (0..1) para CPU y memoria.
type Sample struct {
	CPU float64
	Mem float64
}

// Sampler guarda una ventana fija de muestras para los sparklines.
type Sampler struct {
	window  int
	items   []Sample
	lastCPU uint64
}

// NewSampler crea un sampler con ventana de n muestras.
func NewSampler(n int) *Sampler {
	if n < 2 {
		n = 2
	}
	return &Sampler{window: n}
}

// Capture lee los cgroups de los goblins y agrega una muestra.
func (s *Sampler) Capture(goblins []Goblin) {
	var cpuNow, memNow uint64
	for _, g := range goblins {
		cpuNow += readUint(g.CGroup, "cpu.stat", "usage_usec")
		memNow += readUint(g.CGroup, "memory.current", "")
	}
	cpu := 0.0
	if s.lastCPU > 0 && cpuNow > s.lastCPU {
		// Delta de CPU por tick, normalizado contra un techo de 1 core.
		cpu = float64(cpuNow-s.lastCPU) / 2_000_000.0 // 2s * 1 core
		if cpu > 1 {
			cpu = 1
		}
	}
	s.lastCPU = cpuNow
	// Memoria normalizada contra 4G (techo ilustrativo del dashboard).
	mem := float64(memNow) / float64(4*1024*1024*1024)
	if mem > 1 {
		mem = 1
	}
	s.items = append(s.items, Sample{CPU: cpu, Mem: mem})
	if len(s.items) > s.window {
		s.items = s.items[len(s.items)-s.window:]
	}
}

// Sparklines devuelve dos strings con los gráficos de CPU y memoria.
func (s *Sampler) Sparklines() (cpu, mem string) {
	return spark(s.items, func(x Sample) float64 { return x.CPU }),
		spark(s.items, func(x Sample) float64 { return x.Mem })
}

func spark(items []Sample, pick func(Sample) float64) string {
	if len(items) == 0 {
		return strings.Repeat(" ", 20)
	}
	var b strings.Builder
	for _, it := range items {
		v := pick(it)
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		idx := int(v * float64(len(sparkChars)-1))
		b.WriteByte(sparkChars[idx])
	}
	return b.String()
}

func readUint(dir, file, key string) uint64 {
	data, err := os.ReadFile(filepath.Join(dir, file)) // #nosec G304
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if key != "" {
			if !strings.HasPrefix(line, key+" ") {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, key))
		}
		if n, err := strconv.ParseUint(strings.TrimSpace(line), 10, 64); err == nil {
			return n
		}
	}
	return 0
}

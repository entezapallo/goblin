// cmd/orb/main.go
//
// orb es el TUI de gestión de goblins: consulta manifiestos y cgroupfs, y
// muta por socket vía pkg/verbs. Layout configurable por archivo, no
// dinámico. Lo esencial: tablas, colores, sparklines y eventos.
//
// Teclas: j/k o ↑/↓ navegar, p pausar, r reanudar, x matar, ? ayuda, q salir.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"goblin/pkg/orb"
	"goblin/pkg/verbs"
)

const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	inverse = "\033[7m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	red     = "\033[31m"
	cyan    = "\033[36m"
	dim     = "\033[2m"
)

type model struct {
	cfg      orb.Config
	goblins  []orb.Goblin
	selected int
	message  string
	events   []orb.Event
	sampler  *orb.Sampler
	help     bool
}

func (m *model) current() *orb.Goblin {
	if len(m.goblins) == 0 {
		return nil
	}
	if m.selected >= len(m.goblins) {
		m.selected = 0
	}
	return &m.goblins[m.selected]
}

func stateColor(state string) string {
	switch state {
	case "running", "success":
		return green
	case "paused", "frozen", "orphaned", "alive":
		return yellow
	case "failed", "killed", "dead", "aborted":
		return red
	default:
		return cyan
	}
}

func humanBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%dG", n/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%dM", n/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%dK", n/(1<<10))
	default:
		return fmt.Sprintf("%d", n)
	}
}

func pad(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}

// panel dibuja una caja con título y contenido recortado a height filas.
func panel(title string, lines []string, height int) string {
	width := 76
	var b strings.Builder
	b.WriteString("╔─ " + bold + title + reset + " ")
	b.WriteString(strings.Repeat("─", width-6-len([]rune(title))))
	b.WriteString("╗\n")
	for i := 0; i < height-2; i++ {
		content := ""
		if i < len(lines) {
			content = lines[i]
		}
		b.WriteString("│ " + visiblePad(content, width-4) + " │\n")
	}
	b.WriteString("╚" + strings.Repeat("─", width-2) + "╝\n")
	return b.String()
}

// visiblePad rellena teniendo en cuenta que los códigos ANSI no ocupan columnas.
func visiblePad(s string, width int) string {
	visible := 0
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		visible++
	}
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

func (m *model) topWidget() string {
	cpuSpark, memSpark := m.sampler.Sparklines()
	var totalWorkers int
	var totalMem uint64
	for _, g := range m.goblins {
		totalWorkers += len(g.Workers)
		for _, w := range g.Workers {
			totalMem += w.MemCurrent
		}
	}
	var b strings.Builder
	b.WriteString(" cpu " + cyan + cpuSpark + reset + "   ")
	b.WriteString("mem " + cyan + memSpark + reset + " " + humanBytes(totalMem) + "\n")
	b.WriteString(dim + fmt.Sprintf(" goblins %d  workers %d  %s", len(m.goblins), totalWorkers, m.message) + reset + "\n")
	return b.String()
}

func (m *model) goblinsWidget() []string {
	var lines []string
	lines = append(lines, dim+" ID                 ESTADO     PID     W   MEM"+reset)
	for i, g := range m.goblins {
		cursor := "  "
		if i == m.selected {
			cursor = inverse + "> " + reset
		}
		line := fmt.Sprintf("%s%-18s %s%-9s%s %-7d %-3d %s",
			cursor, orbShort(g.ID, 18), stateColor(g.State), g.State, reset,
			g.PID, len(g.Workers), g.MemMax)
		lines = append(lines, line)
	}
	if len(m.goblins) == 0 {
		lines = append(lines, "(sin goblins vivos)")
	}
	return lines
}

func (m *model) workersWidget() []string {
	g := m.current()
	if g == nil {
		return []string{"(selecciona un goblin)"}
	}
	var lines []string
	lines = append(lines, dim+fmt.Sprintf(" Workers de %s", orbShort(g.ID, 24))+reset)
	lines = append(lines, dim+" NAME            PID     ESTADO    RSS"+reset)
	for _, w := range g.Workers {
		state := "running"
		if w.Frozen {
			state = "frozen"
		}
		lines = append(lines, fmt.Sprintf(" %-15s %-7d %s%-8s%s %s",
			orbShort(w.Name, 15), w.PID, stateColor(state), state, reset, humanBytes(w.MemCurrent)))
	}
	if len(g.Workers) == 0 {
		lines = append(lines, "(sin workers)")
	}
	return lines
}

func (m *model) eventsWidget() []string {
	if len(m.events) == 0 {
		return []string{"(sin eventos)"}
	}
	var lines []string
	for _, e := range m.events {
		stamp := ""
		if !e.Time.IsZero() {
			stamp = e.Time.Format("15:04:05") + " "
		}
		lines = append(lines, dim+stamp+reset+e.Text)
	}
	return lines
}

func orbShort(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func (m *model) render() {
	var b strings.Builder
	b.WriteString("\033[2J\033[H")
	if m.help {
		b.WriteString(panel("ayuda", []string{
			"j / ↓    siguiente goblin",
			"k / ↑    goblin anterior",
			"p        pausar workers",
			"r        reanudar workers",
			"x        matar workers",
			"?        cerrar ayuda",
			"q        salir",
		}, 10))
		fmt.Print(b.String())
		return
	}

	b.WriteString(m.topWidget())
	for _, name := range m.cfg.Widgets {
		h := m.cfg.Heights[name]
		if h <= 0 {
			continue
		}
		switch name {
		case "goblins":
			b.WriteString(panel("Goblins", m.goblinsWidget(), h))
		case "workers":
			b.WriteString(panel("Workers", m.workersWidget(), h))
		case "events":
			b.WriteString(panel("Eventos", m.eventsWidget(), h))
		}
	}
	b.WriteString("\n " + dim + "[j/k] navegar  [p] pausar  [r] reanudar  [x] matar  [?] ayuda  [q] salir" + reset + "\n")
	fmt.Print(b.String())
}

func (m *model) refresh() {
	gs, err := orb.List()
	if err != nil {
		m.message = "error: " + err.Error()
	} else {
		m.goblins = gs
	}
	m.sampler.Capture(m.goblins)
	m.events = orb.Events("", 20)
}

func (m *model) mutate(verb string) {
	g := m.current()
	if g == nil {
		m.message = "sin goblin seleccionado"
		return
	}
	if len(g.Workers) == 0 {
		m.message = "el goblin no tiene workers"
		return
	}
	for _, w := range g.Workers {
		if _, err := verbs.Mutate(g.ID, verb, w.PID, nil); err != nil {
			m.message = fmt.Sprintf("%s: %v", verb, err)
			return
		}
	}
	m.message = fmt.Sprintf("%s → %d worker(s) de %s", verb, len(g.Workers), g.ID)
	m.refresh()
}

func main() {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "orb requiere una terminal interactiva")
		os.Exit(1)
	}
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "orb: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	m := &model{
		cfg:     orb.LoadConfig(),
		message: "listo",
		sampler: orb.NewSampler(28),
	}
	m.refresh()
	m.render()

	buf := make([]byte, 16)
	lastRefresh := time.Now()
	for {
		_ = os.Stdin.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, _ := os.Stdin.Read(buf)
		if n > 0 {
			switch buf[0] {
			case 'q', 3:
				return
			case '?':
				m.help = !m.help
			case 0x1b:
				if n >= 3 && buf[1] == '[' {
					if buf[2] == 'B' && m.selected < len(m.goblins)-1 {
						m.selected++
					} else if buf[2] == 'A' && m.selected > 0 {
						m.selected--
					}
				}
			case 'j':
				if m.selected < len(m.goblins)-1 {
					m.selected++
				}
			case 'k':
				if m.selected > 0 {
					m.selected--
				}
			case 'p':
				m.mutate("pause")
			case 'r':
				m.mutate("resume")
			case 'x':
				m.mutate("kill")
			}
		}
		if time.Since(lastRefresh) >= 2*time.Second {
			m.refresh()
			lastRefresh = time.Now()
		}
		m.render()
	}
}

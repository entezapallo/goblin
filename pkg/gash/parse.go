// Package gash es un frontend sobre ash: parsea el header declarativo,
// los bloques define…let…tel y grun…nurg, y delega los cuerpos de shell al
// intérprete ash real (349/349 en su suite). No modifica el parser de ash.
package gash

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"goblin/pkg/contract"
)

// Header declarativo del goblin, fijo al nacer.
type Header struct {
	Name     string
	MemoryMB int
	CPU      float64
	Workers  int
	Notify   string
	Orphan   string
	Unborn   string
	Headless string
}

// validPolicy acepta las tres políticas del diseño.
func validPolicy(v string) bool {
	switch v {
	case "kill", "keep", "freeze":
		return true
	}
	return false
}

// Def declara un paso: recursos, reglas y hooks.
type Def struct {
	Name       string
	MemoryMB   int
	CPU        float64
	Workers    int
	Labels     map[string]string
	Rules      []contract.RuleSpec
	OnComplete string
	OnFail     string
	OnExit     string
	Leftover   int
}

// Item es o código shell o un bloque grun.
type Item struct {
	Shell string
	Step  *StepItem
}

// StepItem es un bloque grun resuelto a su def.
type StepItem struct {
	Def    string
	Body   string
	Labels map[string]string
}

// Script es el resultado del parseo.
type Script struct {
	Header Header
	Defs   map[string]*Def
	Items  []Item
}

// ParseFile lee y parsea un .gsh.
func ParseFile(path string) (*Script, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- archivo pedido por el usuario
	if err != nil {
		return nil, err
	}
	return Parse(string(data))
}

// Parse parsea el texto de un .gsh.
func Parse(src string) (*Script, error) {
	s := &Script{Defs: make(map[string]*Def)}
	lines := strings.Split(src, "\n")

	var shell []string
	flushShell := func() {
		if len(shell) == 0 {
			return
		}
		s.Items = append(s.Items, Item{Shell: strings.Join(shell, "\n") + "\n"})
		shell = nil
	}

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		switch {
		case isHeaderVerb(fields[0]):
			flushShell()
			if err := parseHeaderVerb(s, fields); err != nil {
				return nil, fmt.Errorf("header: %w", err)
			}

		case fields[0] == "define" || fields[0] == "def":
			flushShell()
			if len(fields) < 2 {
				return nil, fmt.Errorf("define sin nombre")
			}
			def := &Def{Name: fields[1], Labels: make(map[string]string)}
			i++
			if i < len(lines) && strings.TrimSpace(lines[i]) == "let" {
				i++
			}
			for ; i < len(lines); i++ {
				body := strings.TrimSpace(lines[i])
				if body == "tel" {
					break
				}
				if body == "" || strings.HasPrefix(body, "#") {
					continue
				}
				if err := parseDefLine(def, body); err != nil {
					return nil, fmt.Errorf("define %s: %w", def.Name, err)
				}
			}
			s.Defs[def.Name] = def

		case fields[0] == "grun":
			flushShell()
			if len(fields) < 2 {
				return nil, fmt.Errorf("grun sin nombre de paso")
			}
			step := &StepItem{Def: strings.TrimSuffix(fields[1], ":"), Labels: make(map[string]string)}
			for _, kv := range fields[2:] {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return nil, fmt.Errorf("label inválido: %s", kv)
				}
				step.Labels[k] = v
			}
			var body []string
			for i++; i < len(lines); i++ {
				if strings.TrimSpace(lines[i]) == "nurg" {
					break
				}
				body = append(body, lines[i])
			}
			step.Body = strings.Join(body, "\n") + "\n"
			s.Items = append(s.Items, Item{Step: step})

		default:
			shell = append(shell, lines[i])
		}
	}
	flushShell()
	return s, nil
}

func isHeaderVerb(v string) bool {
	switch v {
	case "name", "resources", "notify", "on_orphan", "on_unborn", "on_headless":
		return true
	}
	return false
}

func parseHeaderVerb(s *Script, f []string) error {
	switch f[0] {
	case "name":
		if len(f) < 2 {
			return fmt.Errorf("name sin valor")
		}
		s.Header.Name = f[1]
	case "resources":
		for _, kv := range f[1:] {
			if err := applyResource(&s.Header.MemoryMB, &s.Header.CPU, &s.Header.Workers, kv); err != nil {
				return err
			}
		}
	case "notify":
		s.Header.Notify = strings.Join(f[1:], " ")
	case "on_orphan", "on_unborn", "on_headless":
		if len(f) < 2 {
			return fmt.Errorf("%s sin valor", f[0])
		}
		if !validPolicy(f[1]) {
			return fmt.Errorf("%s: política inválida %q (keep|kill|freeze)", f[0], f[1])
		}
		switch f[0] {
		case "on_orphan":
			s.Header.Orphan = f[1]
		case "on_unborn":
			s.Header.Unborn = f[1]
		case "on_headless":
			s.Header.Headless = f[1]
		}
	}
	return nil
}

func parseDefLine(d *Def, line string) error {
	f := strings.Fields(line)
	// Los recursos vienen como `clave=valor` en una sola palabra.
	if strings.Contains(line, "=") && (strings.HasPrefix(line, "memory") ||
		strings.HasPrefix(line, "cpu") || strings.HasPrefix(line, "workers") ||
		strings.HasPrefix(line, "pids")) {
		return applyResource(&d.MemoryMB, &d.CPU, &d.Workers, line)
	}
	switch f[0] {
	case "labels":
		for _, kv := range f[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("label inválido: %s", kv)
			}
			d.Labels[k] = v
		}
	case "fail_when", "abort_when", "notify_when":
		spec, err := parseRule(line)
		if err != nil {
			return err
		}
		d.Rules = append(d.Rules, spec)
	case "on_job_exit":
		d.OnComplete = strings.Join(f[1:], " ")
	case "on_error":
		d.OnFail = strings.Join(f[1:], " ")
	case "on_exit":
		if len(f) < 2 {
			return fmt.Errorf("on_exit sin valor")
		}
		d.OnExit = f[1]
	case "on_leftover":
		if len(f) >= 3 && f[1] == "term" {
			n, err := strconv.Atoi(f[2])
			if err != nil {
				return err
			}
			d.Leftover = n
		}
	default:
		return fmt.Errorf("directiva desconocida: %s", f[0])
	}
	return nil
}

// parseMemoryMB convierte 512, 2G o 1024K a megabytes.
func parseMemoryMB(v string) (int, error) {
	mult := 1
	num := v
	switch {
	case strings.HasSuffix(v, "G"), strings.HasSuffix(v, "g"):
		mult, num = 1024, v[:len(v)-1]
	case strings.HasSuffix(v, "K"), strings.HasSuffix(v, "k"):
		mult, num = 0, v[:len(v)-1] // fracción de MB; se acepta como 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(num))
	if err != nil {
		return 0, err
	}
	if mult == 0 {
		return n / 1024, nil
	}
	return n * mult, nil
}

func applyResource(mem *int, cpu *float64, workers *int, kv string) error {
	k, v, ok := strings.Cut(kv, "=")
	if !ok {
		return fmt.Errorf("recurso inválido: %s", kv)
	}
	switch k {
	case "memory":
		n, err := parseMemoryMB(v)
		if err != nil {
			return err
		}
		*mem = n
	case "cpu":
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return err
		}
		*cpu = n
	case "pids":
		// `pids` se traduce a workers aproximado (2 procesos por worker).
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		*workers = n / 2
	case "workers":
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		*workers = n
	default:
		return fmt.Errorf("recurso desconocido: %s", k)
	}
	return nil
}

// extractBraced extrae el contenido de la primera llave balanceada.
func extractBraced(s string) (string, error) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", fmt.Errorf("la regla debe usar llaves: %s", s)
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start+1 : i], nil
			}
		}
	}
	return "", fmt.Errorf("llave sin cerrar: %s", s)
}

// setLegacyFields llena Fact/Op/Value para expresiones simples, de modo que
// el formato anterior siga siendo legible desde herramientas y tests.
func setLegacyFields(spec *contract.RuleSpec, e contract.Expr) {
	spec.Name = e.Op
	switch e.Op {
	case "match":
		spec.Fact = "text"
		spec.Value = e.Value
		spec.Name = "match:" + e.Value
	case "oom", "all", "extreme":
		spec.Fact = e.Op
	case "gte", "gt", "lte", "lt", "eq", "neq":
		spec.Op = e.Op
		spec.Fact = e.Fact
		spec.Value = e.Value
	}
}

// ParseRule parsea una línea de regla. Exportado para tests y herramientas.
func ParseRule(line string) (contract.RuleSpec, error) {
	return parseRule(line)
}

// parseRule parsea `abort_when { <expr> } [then notify via ...]`.
func parseRule(line string) (contract.RuleSpec, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return contract.RuleSpec{}, fmt.Errorf("regla vacía")
	}
	spec := contract.RuleSpec{Kind: fields[0]}

	expression, err := extractBraced(line)
	if err != nil {
		return spec, err
	}
	e, err := ParseExpr(expression)
	if err != nil {
		return spec, err
	}
	spec.Expr = e
	setLegacyFields(&spec, e)

	// `then notify via <canal> [data=<valor>]` se traduce a datos para
	// pkg/notify, no a un comando shell.
	if idx := strings.Index(line, " then "); idx >= 0 {
		clause := strings.Fields(strings.TrimSpace(line[idx+len(" then "):]))
		for i, tok := range clause {
			if tok == "via" && i+1 < len(clause) {
				spec.Channel = clause[i+1]
			}
			if strings.HasPrefix(tok, "data=") {
				spec.Data = strings.TrimPrefix(tok, "data=")
			}
		}
		if spec.Channel == "" && len(clause) > 0 {
			spec.Channel = clause[0]
		}
	}
	return spec, nil
}

package contract

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RuleSpec es la forma serializable de una Rule (las Rules son funciones y
// no pueden viajar por NDJSON). gash las construye y el goblin las compila.
type RuleSpec struct {
	Kind    string `json:"kind"` // fail_when | abort_when | notify_when
	Name    string `json:"name"`
	Fact    string `json:"fact"` // text | oom | failed_count | time | mem_over
	Op      string `json:"op,omitempty"`
	Value   string `json:"value,omitempty"`
	Channel string `json:"channel,omitempty"` // destino de notify_when
	Data    string `json:"data,omitempty"`    // payload, o "log" para stderr_tail
	Expr    Expr   `json:"expr,omitempty"`    // AST completo (cuando no es simple)
}

// Compile convierte una spec en una Rule ejecutable.
func (s RuleSpec) Compile() (Rule, error) {
	if s.Expr.Op != "" {
		return CompileExpr(s)
	}
	kind := RuleKind(s.Kind)
	switch kind {
	case RuleFail, RuleAbort, RuleNotify:
	default:
		return Rule{}, fmt.Errorf("regla desconocida: %s", s.Kind)
	}
	name := s.Name
	if name == "" {
		name = s.Fact + ":" + s.Value
	}
	switch s.Fact {
	case "text":
		return TextRule(kind, name, s.Value), nil
	case "oom":
		return OOMRule(kind, name), nil
	case "failed_count":
		n, err := strconv.Atoi(s.Value)
		if err != nil {
			return Rule{}, fmt.Errorf("failed_count inválido: %s", s.Value)
		}
		return FailedCountRule(kind, name, n), nil
	case "time":
		d, err := time.ParseDuration(s.Value)
		if err != nil {
			return Rule{}, fmt.Errorf("duración inválida: %s", s.Value)
		}
		return TimeoutRule(kind, name, d), nil
	case "mem_over":
		b, err := parseBytes(s.Value)
		if err != nil {
			return Rule{}, err
		}
		return MemOverRule(kind, name, b), nil
	default:
		return Rule{}, fmt.Errorf("hecho desconocido: %s", s.Fact)
	}
}

// CompileAll compila una lista de specs.
func CompileAll(specs []RuleSpec) ([]Rule, error) {
	rules := make([]Rule, 0, len(specs))
	for _, s := range specs {
		r, err := s.Compile()
		if err != nil {
			return nil, err
		}
		r.Channel = s.Channel
		r.Data = s.Data
		rules = append(rules, r)
	}
	return rules, nil
}

func parseBytes(v string) (uint64, error) {
	if v == "" {
		return 0, fmt.Errorf("mem_over sin valor")
	}
	mult := uint64(1)
	switch v[len(v)-1] {
	case 'K', 'k':
		mult = 1024
	case 'M', 'm':
		mult = 1024 * 1024
	case 'G', 'g':
		mult = 1024 * 1024 * 1024
	}
	num := v
	if mult != 1 {
		num = v[:len(v)-1]
	}
	n, err := strconv.ParseUint(strings.TrimSpace(num), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("mem_over inválido: %s", v)
	}
	return n * mult, nil
}

package contract

import (
	"fmt"
	"strconv"
	"strings"
)

// Expr es un nodo del AST de una condición. Es serializable para viajar en
// el Event de submit; el actor lo compila a un predicado sobre Facts.
type Expr struct {
	Op     string `json:"op"`               // and|or|xor|not|match|oom|all|extreme|gte|...
	Args   []Expr `json:"args,omitempty"`   // hijos para operadores lógicos
	Fact   string `json:"fact,omitempty"`   // failed_count|oom_count|time|mempeak|memmean|filesize|min|max
	Value  string `json:"value,omitempty"`  // patrón o número con sufijo
	Level  string `json:"level,omitempty"`  // worker|step|goblin (para mempeak/memmean)
	Stream string `json:"stream,omitempty"` // stdout|stderr (para match)
}

// Eval evalúa la expresión sobre un conjunto de hechos.
func (e Expr) Eval(f Facts) bool {
	switch e.Op {
	case "and":
		if len(e.Args) == 0 {
			return false
		}
		for _, a := range e.Args {
			if !a.Eval(f) {
				return false
			}
		}
		return true
	case "or":
		for _, a := range e.Args {
			if a.Eval(f) {
				return true
			}
		}
		return false
	case "xor":
		trueCount := 0
		for _, a := range e.Args {
			if a.Eval(f) {
				trueCount++
			}
		}
		return trueCount == 1
	case "not":
		return len(e.Args) > 0 && !e.Args[0].Eval(f)
	case "all":
		return f.WorkerCount > 0 && f.FailedCount == f.WorkerCount
	case "extreme":
		// Heurística fija del diseño: ≥40% severos o ≥60% fallidos.
		if f.WorkerCount == 0 {
			return false
		}
		return float64(f.SevereCount)/float64(f.WorkerCount) >= 0.4 ||
			float64(f.FailedCount)/float64(f.WorkerCount) >= 0.6
	case "oom":
		return f.OOM || f.OOMCount > 0
	case "match":
		pattern := e.Value
		if pattern == "" {
			return false
		}
		switch e.Stream {
		case "stdout":
			return strings.Contains(f.StdoutTail, pattern)
		case "stderr":
			return strings.Contains(f.StderrTail, pattern)
		default:
			return strings.Contains(f.Text, pattern)
		}
	case "gte", "gt", "lte", "lt", "eq", "neq":
		left := factNumber(f, e.Fact, e.Level)
		right := parseNumber(e.Value)
		switch e.Op {
		case "gte":
			return left >= right
		case "gt":
			return left > right
		case "lte":
			return left <= right
		case "lt":
			return left < right
		case "eq":
			return left == right
		case "neq":
			return left != right
		}
	}
	return false
}

// factNumber resuelve el valor numérico de un hecho.
func factNumber(f Facts, fact, level string) float64 {
	switch fact {
	case "failed_count":
		return float64(f.FailedCount)
	case "oom_count":
		return float64(f.OOMCount)
	case "time":
		return f.Elapsed.Seconds()
	case "mempeak":
		switch level {
		case "worker":
			return float64(maxOf(f.WorkerPeaks))
		case "min":
			return float64(minOf(f.WorkerPeaks))
		default:
			return float64(f.MemPeakBytes)
		}
	case "memmean":
		return float64(f.MemMeanBytes)
	case "min":
		return float64(minOf(f.WorkerPeaks))
	case "max":
		return float64(maxOf(f.WorkerPeaks))
	case "filesize":
		if f.FileSize == nil {
			return 0
		}
		return float64(f.FileSize(level))
	}
	return 0
}

func minOf(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func maxOf(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

// parseNumber acepta sufijos de memoria (K/M/G) y tiempo (s/m/h).
func parseNumber(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	mult := 1.0
	switch s[len(s)-1] {
	case 'K', 'k':
		mult, s = 1024, s[:len(s)-1]
	case 'M', 'm':
		// 'm' puede ser minutos (tiempo) o megabytes (memoria); el valor
		// numérico con sufijo m se interpreta como minutos para time.
		mult, s = 60, s[:len(s)-1]
	case 'G', 'g':
		mult, s = 1024*1024*1024, s[:len(s)-1]
	case 's':
		mult, s = 1, s[:len(s)-1]
	case 'h':
		mult, s = 3600, s[:len(s)-1]
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return n * mult
}

// CompileExpr compila una Expr en una Rule con la spec asociada.
func CompileExpr(spec RuleSpec) (Rule, error) {
	kind := RuleKind(spec.Kind)
	switch kind {
	case RuleFail, RuleAbort, RuleNotify:
	default:
		return Rule{}, fmt.Errorf("regla desconocida: %s", spec.Kind)
	}
	if spec.Expr.Op == "" {
		return Rule{}, fmt.Errorf("expresión vacía")
	}
	expr := spec.Expr // copia para el closure
	return Rule{
		Kind: kind, Name: spec.Name, Channel: spec.Channel, Data: spec.Data,
		When: func(f Facts) bool { return expr.Eval(f) },
	}, nil
}

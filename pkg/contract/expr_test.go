package contract_test

import (
	"testing"
	"time"

	"goblin/pkg/contract"
)

func baseFacts() contract.Facts {
	return contract.Facts{
		WorkerCount: 3, FailedCount: 2, SevereCount: 1, OOMCount: 1,
		MemPeakBytes: 1024, MemMeanBytes: 512,
		WorkerPeaks: []uint64{100, 200, 300},
		Elapsed:     2 * time.Minute,
		StdoutTail:  "hello stdout",
		StderrTail:  "CUDA out of memory",
		Text:        "hello stdout\nCUDA out of memory",
	}
}

func eval(e contract.Expr, f contract.Facts) bool {
	return e.Eval(f)
}

func TestExprLogic(t *testing.T) {
	f := baseFacts()
	cases := []struct {
		name string
		expr contract.Expr
		want bool
	}{
		{"all false", contract.Expr{Op: "all"}, false},
		{"extreme true (2/3 failed)", contract.Expr{Op: "extreme"}, true},
		{"oom true", contract.Expr{Op: "oom"}, true},
		{"match stdout", contract.Expr{Op: "match", Value: "hello", Stream: "stdout"}, true},
		{"match stderr missing", contract.Expr{Op: "match", Value: "hello", Stream: "stderr"}, false},
		{"gte failed_count", contract.Expr{Op: "gte", Fact: "failed_count", Value: "2"}, true},
		{"lt failed_count", contract.Expr{Op: "lt", Fact: "failed_count", Value: "2"}, false},
		{"and", contract.Expr{Op: "and", Args: []contract.Expr{
			{Op: "gte", Fact: "failed_count", Value: "2"},
			{Op: "match", Value: "CUDA", Stream: "stderr"},
		}}, true},
		{"or", contract.Expr{Op: "or", Args: []contract.Expr{
			{Op: "eq", Fact: "oom_count", Value: "0"},
			{Op: "match", Value: "CUDA", Stream: "stderr"},
		}}, true},
		{"xor", contract.Expr{Op: "xor", Args: []contract.Expr{
			{Op: "oom"}, {Op: "all"},
		}}, true},
		{"not", contract.Expr{Op: "not", Args: []contract.Expr{{Op: "all"}}}, true},
		{"min worker peaks", contract.Expr{Op: "lte", Fact: "min", Value: "100"}, true},
		{"max worker peaks", contract.Expr{Op: "gte", Fact: "max", Value: "300"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := eval(c.expr, f); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestExprFileSize(t *testing.T) {
	f := baseFacts()
	f.FileSize = func(path string) int64 {
		if path == "out.txt" {
			return 2048
		}
		return 0
	}
	if !eval(contract.Expr{Op: "gte", Fact: "filesize", Level: "out.txt", Value: "1K"}, f) {
		t.Fatal("filesize no evaluó true")
	}
	if eval(contract.Expr{Op: "gte", Fact: "filesize", Level: "missing", Value: "1K"}, f) {
		t.Fatal("filesize de archivo inexistente dio true")
	}
}

func TestExprCompile(t *testing.T) {
	spec := contract.RuleSpec{
		Kind: "abort_when",
		Expr: contract.Expr{Op: "and", Args: []contract.Expr{
			{Op: "gte", Fact: "failed_count", Value: "2"},
			{Op: "match", Value: "CUDA", Stream: "stderr"},
		}},
	}
	rule, err := spec.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if !rule.When(baseFacts()) {
		t.Fatal("regla compilada no disparó")
	}
}

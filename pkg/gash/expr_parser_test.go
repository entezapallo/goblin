package gash_test

import (
	"testing"

	"goblin/pkg/contract"
	"goblin/pkg/gash"
)

func TestParseComplexRules(t *testing.T) {
	cases := []struct {
		line    string
		wantOp  string
		wantLen int
	}{
		{"abort_when {and {gte failed_count 2} {match 'CUDA out of memory' in stderr}}", "and", 2},
		{"fail_when {or {extreme} {gte time 30m}}", "or", 2},
		{"notify_when {not {eq oom_count 0}} then notify via email data=log", "not", 1},
		{"abort_when {gte filesize(out.txt) 1M}", "gte", 0},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			spec, err := gash.ParseRule(c.line)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if spec.Expr.Op != c.wantOp || len(spec.Expr.Args) != c.wantLen {
				t.Fatalf("expr=%+v want op=%s len=%d", spec.Expr, c.wantOp, c.wantLen)
			}
			if _, err := spec.Compile(); err != nil {
				t.Fatalf("compile: %v", err)
			}
		})
	}
}

func TestParseRuleKeepsLegacyFields(t *testing.T) {
	spec, err := gash.ParseRule("abort_when {match 'boom' in stderr}")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Fact != "text" || spec.Value != "boom" {
		t.Fatalf("legacy fields: %+v", spec)
	}
}

func TestExprFactFileSize(t *testing.T) {
	spec, err := gash.ParseRule("fail_when {gte filesize(out.txt) 1M}")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Expr.Fact != "filesize" || spec.Expr.Level != "out.txt" {
		t.Fatalf("filesize expr: %+v", spec.Expr)
	}
}

func TestComplexRulesThroughParser(t *testing.T) {
	src := `name test
define job
let
  abort_when {and {gte failed_count 2} {match 'boom' in stderr}}
  notify_when {not {eq oom_count 0}} then notify via email data=log
tel
grun job
  echo hi
nurg
`
	script, err := gash.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	def := script.Defs["job"]
	if def == nil || len(def.Rules) != 2 {
		t.Fatalf("def=%+v", def)
	}
	if def.Rules[0].Expr.Op != "and" || def.Rules[1].Expr.Op != "not" {
		t.Fatalf("rules=%+v", def.Rules)
	}
	if def.Rules[1].Channel != "email" || def.Rules[1].Data != "log" {
		t.Fatalf("notify rule=%+v", def.Rules[1])
	}
	var _ contract.RuleSpec
}

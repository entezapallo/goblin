package gash_test

import (
	"testing"

	"goblin/pkg/contract"
	"goblin/pkg/gash"
)

const sample = `name demo
resources memory=2G cpu=1 workers=2
notify email=a@b.c
on_orphan kill

define align
let
  memory=512
  cpu=1
  workers=2
  labels part=1
  abort_when {match 'CUDA out of memory' in stderr}
  fail_when {gte failed_count 2}
  notify_when {gt time 30s}
  on_job_exit echo OK
  on_error echo FAIL
  on_exit kill
  on_leftover term 5
tel

echo before
grun align part=2
  echo working
nurg
echo after
`

func TestParse(t *testing.T) {
	s, err := gash.Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if s.Header.Name != "demo" || s.Header.MemoryMB != 2048 || s.Header.Orphan != "kill" {
		t.Fatalf("header=%+v", s.Header)
	}
	def := s.Defs["align"]
	if def == nil {
		t.Fatal("define align no encontrado")
	}
	if def.MemoryMB != 512 || def.Workers != 2 || def.Leftover != 5 {
		t.Fatalf("def=%+v", def)
	}
	if len(def.Rules) != 3 {
		t.Fatalf("rules=%+v", def.Rules)
	}
	r0 := def.Rules[0]
	if r0.Kind != "abort_when" || r0.Fact != "text" || r0.Value != "CUDA out of memory" {
		t.Fatalf("rule0=%+v", r0)
	}
	if def.OnComplete != "echo OK" || def.OnFail != "echo FAIL" {
		t.Fatalf("hooks=%+v %+v", def.OnComplete, def.OnFail)
	}
	if len(s.Items) != 3 {
		t.Fatalf("items=%d", len(s.Items))
	}
	if s.Items[1].Step == nil || s.Items[1].Step.Def != "align" {
		t.Fatalf("item1=%+v", s.Items[1])
	}
	if s.Items[1].Step.Labels["part"] != "2" {
		t.Fatalf("labels=%+v", s.Items[1].Step.Labels)
	}
	if _, err := contract.CompileAll(def.Rules); err != nil {
		t.Fatalf("compilar reglas: %v", err)
	}
}

func TestParsePolicies(t *testing.T) {
	s, err := gash.Parse(`name demo
on_orphan freeze
on_unborn keep
on_headless kill
define j
let
tel
`)
	if err != nil {
		t.Fatal(err)
	}
	if s.Header.Orphan != "freeze" || s.Header.Unborn != "keep" || s.Header.Headless != "kill" {
		t.Fatalf("header=%+v", s.Header)
	}
	if _, err := gash.Parse("on_unborn bomba\n"); err == nil {
		t.Fatal("política inválida aceptada")
	}
	if _, err := gash.Parse("on_headless\n"); err == nil {
		t.Fatal("política sin valor aceptada")
	}
}

func TestParseUnsupportedFails(t *testing.T) {
	_, err := gash.Parse("define x\nlet\n  memroy=2G\ntel\n")
	if err == nil {
		t.Fatal("se esperaba error por directiva desconocida")
	}
}

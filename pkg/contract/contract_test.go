package contract_test

import (
	"strings"
	"testing"
	"time"

	"goblin/pkg/contract"
)

func TestStepSuccessAndHook(t *testing.T) {
	s := contract.NewStep("step-1", nil, "echo OK", "echo FAIL", nil, nil)
	s.State = contract.StepStarting
	s.AddWorker(100, "g/step-1/w-1")
	if s.State != contract.StepActive {
		t.Fatalf("estado=%s, quería active", s.State)
	}
	s.OnWorkerExit(100, 0, false, time.Now(), 0, false)
	if !s.AllWorkersDone() {
		t.Fatal("el worker terminó pero no se marca done")
	}
	effects := s.Close(time.Now(), 0, false)
	if len(effects) != 2 || effects[0].Outcome != contract.OutcomeSuccess {
		t.Fatalf("effects=%+v", effects)
	}
	if effects[1].Kind != "hook" || !strings.Contains(effects[1].Command, "OK") {
		t.Fatalf("hook=%+v", effects[1])
	}
}

func TestFailWhenAndFailHook(t *testing.T) {
	rules := []contract.Rule{contract.FailedCountRule(contract.RuleFail, "muchos-fallos", 1)}
	s := contract.NewStep("step-1", rules, "echo OK", "echo FAIL", nil, nil)
	s.AddWorker(100, "g/step-1/w-1")
	s.OnWorkerExit(100, 2, false, time.Now(), 0, false)
	effects := s.Close(time.Now(), 0, false)
	if effects[0].Outcome != contract.OutcomeFailed {
		t.Fatalf("outcome=%s", effects[0].Outcome)
	}
	if !strings.Contains(effects[1].Command, "FAIL") {
		t.Fatalf("hook=%+v", effects[1])
	}
}

func TestAbortWhenTextFiresOnce(t *testing.T) {
	rules := []contract.Rule{contract.TextRule(contract.RuleAbort, "oom-text", "out of memory")}
	s := contract.NewStep("step-1", rules, "", "", nil, nil)
	s.AddWorker(100, "g/step-1/w-1")
	s.AddWorker(101, "g/step-1/w-2")

	s.OnOutput("CUDA out of memory\n")
	effects := s.Tick(time.Now(), 0, false)
	if len(effects) != 1 || effects[0].Kind != "abort" {
		t.Fatalf("effects=%+v", effects)
	}
	if s.State != contract.StepDraining {
		t.Fatalf("estado=%s", s.State)
	}
	// Segundo tick: no debe repetir el abort.
	if effects := s.Tick(time.Now(), 0, false); len(effects) != 0 {
		t.Fatalf("abort se disparó dos veces: %+v", effects)
	}
}

func TestNotifyCooldown(t *testing.T) {
	rules := []contract.Rule{{
		Kind:    contract.RuleNotify,
		Name:    "notificar",
		When:    func(f contract.Facts) bool { return true },
		Channel: "email",
		Data:    "alerta",
	}}
	s := contract.NewStep("step-1", rules, "", "", nil, nil)
	s.AddWorker(100, "g/step-1/w-1")
	now := time.Now()
	if got := s.Tick(now, 0, false); len(got) != 1 {
		t.Fatalf("primer notify=%+v", got)
	}
	if got := s.Tick(now.Add(time.Second), 0, false); len(got) != 0 {
		t.Fatalf("notify dentro del enfriamiento=%+v", got)
	}
	if got := s.Tick(now.Add(2*time.Minute), 0, false); len(got) != 1 {
		t.Fatalf("notify después del enfriamiento=%+v", got)
	}
}

func TestOOMAndTimeoutFacts(t *testing.T) {
	rules := []contract.Rule{
		contract.OOMRule(contract.RuleAbort, "oom"),
		contract.TimeoutRule(contract.RuleNotify, "tiempo", 50*time.Millisecond),
	}
	s := contract.NewStep("step-1", rules, "", "", nil, nil)
	s.AddWorker(100, "g/step-1/w-1")
	// Timeout aún no: el paso recién empieza.
	s.OnWorkerExit(100, 0, true, time.Now(), 0, true)
	f := s.Facts(time.Now(), 0, true)
	if !f.OOM || f.FailedCount != 1 {
		t.Fatalf("facts=%+v", f)
	}
	effects := s.Close(time.Now(), 0, true)
	if effects[0].Outcome != contract.OutcomeAborted {
		t.Fatalf("outcome=%s", effects[0].Outcome)
	}
}

func TestMemOverRule(t *testing.T) {
	s := contract.NewStep("step-1", []contract.Rule{
		contract.MemOverRule(contract.RuleAbort, "mem", 1024),
	}, "", "", nil, nil)
	s.AddWorker(100, "g/step-1/w-1")
	if got := s.Tick(time.Now(), 2048, true); len(got) != 1 {
		t.Fatalf("mem_over no disparó: %+v", got)
	}
}

package gutils_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goblin/pkg/contract"
	"goblin/pkg/gutils"
)

// Cubre notify_when con canal, labels en hooks, struct de fallo con
// status.json y stderr_tail, e historial al cerrar el paso.
func TestObservability(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere cgroupfs real")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	gg := filepath.Join(root, "scripts", "gg")
	guard := buildGuard(t, root)

	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	home := t.TempDir()
	t.Setenv("HOME", home)

	ancestorOut, err := exec.Command(gg, "discover").Output()
	if err != nil {
		t.Fatal(err)
	}
	ancestor := strings.TrimSpace(string(ancestorOut))
	base := filepath.Join(ancestor, "goblin")
	id := fmt.Sprintf("obs-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		kill := exec.Command(gg, "kill", id) // #nosec G204
		kill.Env = append(os.Environ(), "GOBLIN_CGROUP="+base)
		_ = kill.Run()
		rm := exec.Command(gg, "remove", id) // #nosec G204
		rm.Env = append(os.Environ(), "GOBLIN_CGROUP="+base)
		_ = rm.Run()
	})
	create := exec.Command(gg, "create", id) // #nosec G204
	create.Env = append(os.Environ(), "GOBLIN_CGROUP="+base)
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("gg create: %v: %s", err, out)
	}

	labelFile := filepath.Join(t.TempDir(), "label.txt")
	socket := filepath.Join(runtimeDir, "goblin", id+".sock")
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer writeEnd.Close()
	cfg := gutils.Config{
		ID: id, SocketPath: socket, CGroupPath: filepath.Join(base, id),
		DeathPipeFD: int(readEnd.Fd()), OwnerPID: os.Getpid(),
		GGPath: gg, GuardBin: guard, RuntimeDir: runtimeDir,
	}
	done := make(chan error, 1)
	go func() { done <- gutils.Run(cfg) }()
	waitFor(t, "manifiesto", func() bool {
		_, err := os.Stat(filepath.Join(runtimeDir, "goblin", id+".json"))
		return err == nil
	})
	waitFor(t, "socket", func() bool {
		_, err := os.Stat(socket)
		return err == nil
	})

	script := `echo BAD >&2; printf '{"reason":"x"}' > "$GOBLIN_WORKDIR/status.json"; exit 1`
	resp, err := gutils.Call(socket, gutils.Event{
		Type:   "submit",
		Cmd:    []string{"sh", "-c", script},
		Labels: map[string]string{"part": "2"},
		Rules: []contract.RuleSpec{{
			Kind: "notify_when", Fact: "text", Value: "BAD",
			Channel: "email", Data: "alert",
		}},
		OnFail: fmt.Sprintf("printf %%s \"$GOBLIN_LABEL_part\" > %s", labelFile),
	})
	if err != nil || resp.Type != "submit" {
		t.Fatalf("submit: %+v err=%v", resp, err)
	}

	waitFor(t, "paso cerrado", func() bool {
		r, err := gutils.Call(socket, gutils.Event{Type: "list"})
		if err != nil {
			return false
		}
		for _, info := range r.Steps {
			if info.State == "closed" {
				return true
			}
		}
		return false
	})

	// 1. Label disponible en el entorno del hook.
	data, err := os.ReadFile(labelFile)
	if err != nil || string(data) != "2" {
		t.Fatalf("label en hook=%q err=%v", data, err)
	}

	// 2. Notificación con canal y payload.
	notifications, err := os.ReadFile(filepath.Join(runtimeDir, "goblin", "notifications.jsonl"))
	if err != nil || !strings.Contains(string(notifications), `"channel":"email"`) ||
		!strings.Contains(string(notifications), `"data":"alert"`) {
		t.Fatalf("notifications=%q err=%v", notifications, err)
	}

	// 3. Struct de fallo con labels, código, stderr_tail y status.json.
	failures, err := os.ReadFile(filepath.Join(runtimeDir, "goblin", id+".failures.jsonl"))
	if err != nil {
		t.Fatalf("failures: %v", err)
	}
	var failure contract.Failure
	if err := json.Unmarshal(bytes.TrimSpace(failures), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Labels["part"] != "2" || failure.Exit.Code != 1 || failure.Status["reason"] != "x" {
		t.Fatalf("failure=%+v", failure)
	}
	if !strings.Contains(strings.Join(failure.StderrTail, "\n"), "BAD") {
		t.Fatalf("stderr_tail=%v", failure.StderrTail)
	}

	// 4. Historial global (HOME aislado para el test).
	histPath := filepath.Join(home, ".local", "share", "goblin", "history.jsonl")
	waitFor(t, "history.jsonl", func() bool {
		_, err := os.Stat(histPath)
		return err == nil
	})
	hist, _ := os.ReadFile(histPath)
	if !strings.Contains(string(hist), `"state":"failed"`) {
		t.Fatalf("history=%q", hist)
	}

	// 5. step_tree en el historial del goblin.
	goblinLog := filepath.Join(runtimeDir, "goblin", id+".history.jsonl")
	waitFor(t, "step_tree", func() bool {
		data, err := os.ReadFile(goblinLog)
		return err == nil && strings.Contains(string(data), `"event":"step_tree"`)
	})
	treeData, _ := os.ReadFile(goblinLog)
	if !strings.Contains(string(treeData), `"step":"step-1"`) {
		t.Fatalf("step_tree=%q", treeData)
	}

	_ = writeEnd.Close()
	<-done
}

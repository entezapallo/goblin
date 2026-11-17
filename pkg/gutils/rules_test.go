package gutils_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goblin/pkg/contract"
	"goblin/pkg/gutils"
	"goblin/pkg/manifest"
)

// Prueba que una regla abort_when sobre texto really mata el worker y
// dispara el hook on_fail, con el goblin vivo todo el tiempo.
func TestContractRulesEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere cgroupfs real")
	}

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	gg := filepath.Join(root, "scripts", "gg")

	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	out, err := exec.Command(gg, "discover").Output()
	if err != nil {
		t.Fatal(err)
	}
	ancestor := strings.TrimSpace(string(out))
	base := filepath.Join(ancestor, fmt.Sprintf("gg-rules-%d", os.Getpid()))
	t.Setenv("GOBLIN_CGROUP", base)

	const id = "gutils-rules"
	hookFile := filepath.Join(runtimeDir, "hook.txt")
	t.Cleanup(func() {
		_ = exec.Command(gg, "kill", id).Run()
		_ = exec.Command(gg, "remove", id).Run()
		_ = os.RemoveAll(base)
	})
	if out, err := exec.Command(gg, "create", id).CombinedOutput(); err != nil {
		t.Fatalf("gg create: %v: %s", err, out)
	}

	cgroup := filepath.Join(base, id)
	socket := filepath.Join(runtimeDir, "goblin", id+".sock")
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readEnd.Close()

	cfg := gutils.Config{
		ID:          id,
		SocketPath:  socket,
		CGroupPath:  cgroup,
		DeathPipeFD: int(readEnd.Fd()),
		OwnerPID:    os.Getpid(),
		GGPath:      gg,
		Rules: []contract.Rule{
			contract.TextRule(contract.RuleAbort, "oom-text", "ABORT_NOW"),
		},
		OnFail: fmt.Sprintf("printf aborted > %s", hookFile),
	}
	done := make(chan error, 1)
	go func() { done <- gutils.Run(cfg) }()

	waitFor(t, "manifiesto", func() bool {
		_, err := manifest.Read(id)
		return err == nil
	})
	waitFor(t, "socket", func() bool {
		_, err := os.Stat(socket)
		return err == nil
	})

	resp, err := gutils.Call(socket, gutils.Event{
		Type: "submit",
		Cmd:  []string{"sh", "-c", "echo ABORT_NOW; sleep 30"},
	})
	if err != nil || resp.Type != "submit" {
		t.Fatalf("submit: %+v err=%v", resp, err)
	}

	// La regla aborta el paso: el hook on_fail escribe el archivo.
	waitFor(t, "hook on_fail", func() bool {
		data, err := os.ReadFile(hookFile)
		return err == nil && strings.TrimSpace(string(data)) == "aborted"
	})

	// El worker debe haber muerto (cgroup del paso vacío).
	stepProcs := filepath.Join(cgroup, "step-1", "cgroup.procs")
	waitFor(t, "worker abortado", func() bool {
		data, _ := os.ReadFile(stepProcs)
		return strings.TrimSpace(string(data)) == ""
	})

	_ = writeEnd.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout esperando salida del goblin")
	}
}

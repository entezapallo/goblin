package gutils_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goblin/pkg/gutils"
)

// list debe devolver pasos estructurados, no un string colado en Error.
func TestListTyped(t *testing.T) {
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
	out, err := exec.Command(gg, "discover").Output()
	if err != nil {
		t.Fatal(err)
	}
	ancestor := strings.TrimSpace(string(out))
	base := filepath.Join(ancestor, "goblin")
	id := fmt.Sprintf("list-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		summonCleanup(gg, ancestor, id)
	})
	create := exec.Command(gg, "create", id) // #nosec G204
	create.Env = append(os.Environ(), "GOBLIN_CGROUP="+base)
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("gg create: %v: %s", err, out)
	}

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
	waitFor(t, "socket", func() bool {
		_, err := os.Stat(socket)
		return err == nil
	})

	resp, err := gutils.Call(socket, gutils.Event{Type: "submit", Cmd: []string{"sleep", "2"}})
	if err != nil || resp.Type != "submit" {
		t.Fatalf("submit: %+v err=%v", resp, err)
	}

	waitFor(t, "list tipado", func() bool {
		r, err := gutils.Call(socket, gutils.Event{Type: "list"})
		if err != nil || r.Type != "list" {
			return false
		}
		if len(r.Steps) == 0 || r.Steps[0].ID != "step-1" {
			return false
		}
		return r.Steps[0].State == "closed" && r.Steps[0].Outcome == "success" && r.Steps[0].Workers == 1
	})
	// El campo Error ya no se usa para el resumen.
	r, _ := gutils.Call(socket, gutils.Event{Type: "list"})
	if r.Error != "" {
		t.Fatalf("list sigue colando resumen en Error: %q", r.Error)
	}

	_ = writeEnd.Close()
	<-done
}

func summonCleanup(gg, ancestor, id string) {
	base := filepath.Join(ancestor, "goblin")
	for _, verb := range []string{"kill", "remove"} {
		c := exec.Command(gg, verb, id) // #nosec G204
		c.Env = append(os.Environ(), "GOBLIN_CGROUP="+base)
		_ = c.Run()
	}
}

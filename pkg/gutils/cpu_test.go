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

// Terms.CPUCores fraccionario debe llegar a cpu.max como quota/period.
func TestFractionalCPUReachesCgroup(t *testing.T) {
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
	id := fmt.Sprintf("cpu-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() { summonCleanup(gg, ancestor, id) })
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

	resp, err := gutils.Call(socket, gutils.Event{
		Type: "submit", Cmd: []string{"sleep", "2"},
		Terms: gutils.Terms{CPUCores: 1.5},
	})
	if err != nil || resp.Type != "submit" {
		t.Fatalf("submit: %+v err=%v", resp, err)
	}

	waitFor(t, "cpu.max fraccionario", func() bool {
		data, err := os.ReadFile(filepath.Join(base, id, "step-1", "cpu.max"))
		return err == nil && strings.TrimSpace(string(data)) == "150000 100000"
	})

	_ = writeEnd.Close()
	<-done
}

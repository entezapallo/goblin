package gutils_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goblin/pkg/discover"
	"goblin/pkg/gutils"
	"goblin/pkg/manifest"
)

const testID = "gutils-test"

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout esperando: %s", what)
}

func buildGuard(t *testing.T, root string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "gguard")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/gguard")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build gguard: %v: %s", err, out)
	}
	return bin
}

func readEvents(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func TestGoblinLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere cgroupfs real")
	}

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	gg := filepath.Join(root, "scripts", "gg")
	guard := buildGuard(t, root)
	if _, err := os.Stat(gg); err != nil {
		t.Fatalf("gg no encontrado: %v", err)
	}

	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	ancestorOut, err := exec.Command(gg, "discover").Output()
	if err != nil {
		t.Fatalf("gg discover: %v", err)
	}
	ancestor := strings.TrimSpace(string(ancestorOut))
	base := filepath.Join(ancestor, fmt.Sprintf("gg-gutils-%d", os.Getpid()))
	t.Setenv("GOBLIN_CGROUP", base)
	t.Cleanup(func() {
		_ = exec.Command(gg, "kill", testID).Run()
		_ = exec.Command(gg, "remove", testID).Run()
		_ = os.RemoveAll(base)
	})

	if out, err := exec.Command(gg, "create", testID).CombinedOutput(); err != nil {
		t.Fatalf("gg create: %v: %s", err, out)
	}

	cgroup := filepath.Join(base, testID)
	socket := filepath.Join(runtimeDir, "goblin", testID+".sock")
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readEnd.Close()

	cfg := gutils.Config{
		ID:          testID,
		SocketPath:  socket,
		CGroupPath:  cgroup,
		DeathPipeFD: int(readEnd.Fd()),
		OwnerPID:    os.Getpid(),
		GGPath:      gg,
		GuardBin:    guard,
	}
	done := make(chan error, 1)
	go func() { done <- gutils.Run(cfg) }()

	waitFor(t, "manifiesto", func() bool {
		_, err := manifest.Read(testID)
		return err == nil
	})
	waitFor(t, "socket", func() bool {
		_, err := os.Stat(socket)
		return err == nil
	})

	// Direccionamiento por id contra el manifiesto.
	g, err := discover.Resolve(testID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if g.ID != testID || g.CGroup != cgroup {
		t.Fatalf("Resolve devolvió %+v", g)
	}

	// submit: un sleep real dentro del cgroup del worker.
	resp, err := gutils.Call(socket, gutils.Event{Type: "submit", Cmd: []string{"sleep", "30"}})
	if err != nil || resp.Type != "submit" || resp.PID == 0 {
		t.Fatalf("submit: resp=%+v err=%v", resp, err)
	}
	pid := resp.PID
	stepCgroup := filepath.Join(cgroup, "step-1")
	defer func() { _ = exec.Command(gg, "kill", testID+"/step-1").Run() }()

	workerCgroup := filepath.Join(stepCgroup, "w-1")
	waitFor(t, "worker en cgroup", func() bool {
		data, _ := os.ReadFile(filepath.Join(workerCgroup, "cgroup.procs"))
		return strings.Contains(string(data), fmt.Sprint(pid))
	})

	// pause: cgroup.freeze real.
	resp, err = gutils.Call(socket, gutils.Event{Type: "pause", PID: pid})
	if err != nil || resp.State != gutils.StatePaused {
		t.Fatalf("pause: resp=%+v err=%v", resp, err)
	}
	waitFor(t, "frozen", func() bool {
		return strings.Contains(readEvents(t, filepath.Join(workerCgroup, "cgroup.events")), "frozen 1")
	})

	// resume.
	resp, err = gutils.Call(socket, gutils.Event{Type: "resume", PID: pid})
	if err != nil || resp.State != gutils.StateRunning {
		t.Fatalf("resume: resp=%+v err=%v", resp, err)
	}

	// kill: mata el subárbol.
	resp, err = gutils.Call(socket, gutils.Event{Type: "kill", PID: pid})
	if err != nil || resp.State != gutils.StateKilled {
		t.Fatalf("kill: resp=%+v err=%v", resp, err)
	}
	waitFor(t, "worker vacío", func() bool {
		data, _ := os.ReadFile(filepath.Join(workerCgroup, "cgroup.procs"))
		return strings.TrimSpace(string(data)) == ""
	})

	// Muerte del instructor: EOF en el pipe → cascada y salida limpia.
	_ = writeEnd.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run al morir el instructor: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout esperando que el goblin muriera con el instructor")
	}
	if _, err := manifest.Read(testID); err == nil {
		t.Fatal("el manifiesto sobrevivió al goblin")
	}
}

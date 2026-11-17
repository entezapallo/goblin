package gutils_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"goblin/pkg/gutils"
)

// Prueba la garantía dura: si el goblin muere de golpe, el worker muere
// aunque nadie corra código del goblin. El guard del worker tiene un pipe
// cuyo write end solo sostiene el proceso goblin.
func TestGoblinSIGKILLKillsWorker(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere cgroupfs real y SIGKILL")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	build := func(name, pkg string) string {
		t.Helper()
		bin := filepath.Join(binDir, name)
		cmd := exec.Command("go", "build", "-o", bin, pkg)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v: %s", name, err, out)
		}
		return bin
	}
	build("goblin", "./cmd/goblin")
	guard := build("gguard", "./cmd/gguard")
	gg := filepath.Join(root, "scripts", "gg")

	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	ancestorOut, err := exec.Command(gg, "discover").Output()
	if err != nil {
		t.Fatal(err)
	}
	ancestor := strings.TrimSpace(string(ancestorOut))
	base := filepath.Join(ancestor, "goblin")
	id := fmt.Sprintf("liveness-%d-%d", os.Getpid(), time.Now().UnixNano())
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

	socket := filepath.Join(runtimeDir, "goblin", id+".sock")
	liveRead, liveWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer liveWrite.Close()

	cmd := exec.Command(filepath.Join(binDir, "goblin"),
		id, socket, filepath.Join(base, id), "3", strconv.Itoa(os.Getpid()), gg) // #nosec G204
	cmd.Env = append(os.Environ(), "GOBLIN_GUARD="+guard)
	cmd.ExtraFiles = []*os.File{liveRead}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = liveRead.Close()

	waitFor(t, "manifiesto", func() bool {
		_, err := os.Stat(filepath.Join(runtimeDir, "goblin", id+".json"))
		return err == nil
	})

	resp, err := gutils.Call(socket, gutils.Event{Type: "submit", Cmd: []string{"sleep", "60"}})
	if err != nil || resp.Type != "submit" {
		t.Fatalf("submit: %+v err=%v", resp, err)
	}
	guardPID := resp.PID

	// El worker real es hijo del guard.
	waitFor(t, "hijo del guard", func() bool {
		_, err := os.Stat(fmt.Sprintf("/proc/%d", guardPID))
		return err == nil
	})
	var workerPID int
	waitFor(t, "worker en /proc", func() bool {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", guardPID, guardPID))
		if err != nil || len(strings.Fields(string(data))) == 0 {
			return false
		}
		workerPID, _ = strconv.Atoi(strings.Fields(string(data))[0])
		return workerPID > 0
	})

	// Muerte dura del goblin. Nadie ejecuta onDeath.
	if err := syscall.Kill(cmd.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	waitFor(t, "worker muerto tras SIGKILL del goblin", func() bool {
		_, err := os.Stat(fmt.Sprintf("/proc/%d", workerPID))
		return os.IsNotExist(err)
	})
}

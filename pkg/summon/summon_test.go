package summon_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goblin/pkg/gutils"
	"goblin/pkg/summon"
)

func buildTestBinaries(t *testing.T) (gg, goblin, guard string) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	gg = filepath.Join(root, "scripts", "gg")
	binDir := t.TempDir()
	buildGoblin := exec.Command("go", "build", "-o", filepath.Join(binDir, "goblin"), "./cmd/goblin")
	buildGoblin.Dir = root
	if out, err := buildGoblin.CombinedOutput(); err != nil {
		t.Fatalf("build goblin: %v: %s", err, out)
	}
	goblin = filepath.Join(binDir, "goblin")
	guard = filepath.Join(binDir, "gguard")
	cmd := exec.Command("go", "build", "-o", guard, "./cmd/gguard")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build gguard: %v: %s", err, out)
	}
	return gg, goblin, guard
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("timeout esperando: %s", what)
}

func TestSummonRunsJobAndRetires(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere cgroupfs real")
	}
	gg, goblinBin, guardBin := buildTestBinaries(t)
	runtimeDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "marker.txt")
	id := fmt.Sprintf("summon-test-%d", os.Getpid())

	out, err := exec.Command(gg, "discover").Output()
	if err != nil {
		t.Fatal(err)
	}
	ancestor := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		summon.CleanupByName(gg, ancestor, id)
		_ = os.RemoveAll(filepath.Join(ancestor, "goblin", id))
	})

	idBack, err := summon.Launch(summon.Config{
		ID:         id,
		Cmd:        []string{"sh", "-c", fmt.Sprintf("printf hi > %s", marker)},
		GGPath:     gg,
		GoblinBin:  goblinBin,
		GuardBin:   guardBin,
		RuntimeDir: runtimeDir,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if idBack != id {
		t.Fatalf("id=%s, quería %s", idBack, id)
	}

	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "hi" {
		t.Fatalf("marker=%q err=%v", data, err)
	}

	// El paso cerró; al salir Launch cerró el pipe y el goblin se retiró.
	waitFor(t, "manifiesto retirado", func() bool {
		_, err := summon.ReadManifest(runtimeDir, id)
		return err != nil
	})

	// Launch cerró la sesión completa: tampoco debe quedar el cgroup.
	if _, err := os.Stat(filepath.Join(ancestor, "goblin", id)); !os.IsNotExist(err) {
		t.Fatalf("el cgroup del goblin quedó huérfano: %v", err)
	}
}

func TestSummonDetach(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere cgroupfs real")
	}
	gg, goblinBin, guardBin := buildTestBinaries(t)
	runtimeDir := t.TempDir()
	id := fmt.Sprintf("summon-detach-%d", os.Getpid())

	out, err := exec.Command(gg, "discover").Output()
	if err != nil {
		t.Fatal(err)
	}
	ancestor := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		summon.CleanupByName(gg, ancestor, id)
		_ = os.RemoveAll(filepath.Join(ancestor, "goblin", id))
	})

	started := time.Now()
	if _, err := summon.Launch(summon.Config{
		ID:           id,
		Cmd:          []string{"sleep", "30"},
		Detach:       true,
		OrphanPolicy: "kill",
		GGPath:       gg,
		GoblinBin:    goblinBin,
		GuardBin:     guardBin,
		RuntimeDir:   runtimeDir,
	}); err != nil {
		t.Fatalf("Launch detach: %v", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("detach esperó demasiado")
	}

	m, err := summon.ReadManifest(runtimeDir, id)
	if err != nil {
		t.Fatalf("manifiesto: %v", err)
	}
	if m.CGroup == "" || m.Socket == "" {
		t.Fatalf("manifiesto incompleto: %+v", m)
	}

	// death: con política kill mata el worker y retira el manifiesto.
	socket := filepath.Join(runtimeDir, "goblin", id+".sock")
	// readiness: el socket ya debe responder consultas.
	if _, err := gutils.Call(socket, gutils.Event{Type: "list"}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if _, err := gutils.Call(socket, gutils.Event{Type: "death"}); err != nil {
		t.Fatalf("death: %v", err)
	}
	waitFor(t, "manifiesto retirado tras death", func() bool {
		_, err := summon.ReadManifest(runtimeDir, id)
		return err != nil
	})
}

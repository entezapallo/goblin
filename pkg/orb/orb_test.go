package orb_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goblin/pkg/gutils"
	"goblin/pkg/orb"
	"goblin/pkg/summon"
)

func buildBinaries(t *testing.T) (root, gg, goblin, guard string) {
	t.Helper()
	var err error
	root, err = filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	gg = filepath.Join(root, "scripts", "gg")
	binDir := t.TempDir()
	for name, pkg := range map[string]string{"goblin": "./cmd/goblin", "gguard": "./cmd/gguard"} {
		bin := filepath.Join(binDir, name)
		cmd := exec.Command("go", "build", "-o", bin, pkg)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v: %s", name, err, out)
		}
		if name == "goblin" {
			goblin = bin
		} else {
			guard = bin
		}
	}
	return
}

func TestListSeesLiveGoblinAndWorker(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere cgroupfs real")
	}
	_, gg, goblin, guard := buildBinaries(t)
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	out, err := exec.Command(gg, "discover").Output()
	if err != nil {
		t.Fatal(err)
	}
	ancestor := strings.TrimSpace(string(out))
	id := fmt.Sprintf("orb-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		summon.CleanupByName(gg, ancestor, id)
	})

	session, err := summon.StartSession(summon.Config{
		ID: id, GGPath: gg, GoblinBin: goblin, GuardBin: guard,
		RuntimeDir: runtimeDir, OrphanPolicy: "kill",
	})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	defer session.Close()

	resp, err := session.Submit(gutils.Event{Type: "submit", Cmd: []string{"sleep", "30"}})
	if err != nil || resp.Type != "submit" {
		t.Fatalf("submit: %+v err=%v", resp, err)
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		goblins, err := orb.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range goblins {
			if g.ID != id {
				continue
			}
			if len(g.Workers) == 0 {
				break
			}
			if g.CGroup == "" || g.Socket == "" {
				t.Fatalf("goblin incompleto: %+v", g)
			}
			if g.Workers[0].PID == 0 || g.Workers[0].Name == "" {
				t.Fatalf("worker incompleto: %+v", g.Workers[0])
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("orb.List no vio el goblin/worker vivo")
}

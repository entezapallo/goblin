package gash_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goblin/pkg/gash"
	"goblin/pkg/summon"
)

func buildBinaries(t *testing.T) (binDir string) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binDir = t.TempDir()
	for name, pkg := range map[string]string{"goblin": "./cmd/goblin", "gash": "./cmd/gash", "gguard": "./cmd/gguard"} {
		cmd := exec.Command("go", "build", "-o", filepath.Join(binDir, name), pkg)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v: %s", name, err, out)
		}
	}
	return binDir
}

func waitFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && string(data) == want {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("archivo %s no contiene %q", path, want)
}

func TestRunnerEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere cgroupfs real")
	}
	binDir := buildBinaries(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	runtimeDir := t.TempDir()
	workDir := t.TempDir()
	marker := filepath.Join(workDir, "marker.txt")
	hook := filepath.Join(workDir, "hook.txt")

	name := fmt.Sprintf("gash-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	ancestorOut, err := exec.Command(filepath.Join(root, "scripts", "gg"), "discover").Output()
	if err != nil {
		t.Fatal(err)
	}
	ancestor := strings.TrimSpace(string(ancestorOut))
	t.Cleanup(func() {
		summon.CleanupByName(filepath.Join(root, "scripts", "gg"), ancestor, name)
	})

	src := fmt.Sprintf(`name %s
resources memory=256
on_orphan keep

define job
let
  memory=128
  cpu=1
  workers=1
  labels part=1
  on_job_exit printf done > %s
tel

echo start
grun job part=2
  printf worked > %s
nurg
echo end
`, name, hook, marker)

	scriptPath := filepath.Join(workDir, "test.gsh")
	if err := os.WriteFile(scriptPath, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(filepath.Join(binDir, "gash"), scriptPath)
	cmd.Env = append(os.Environ(),
		"GOBLIN_GG="+filepath.Join(root, "scripts", "gg"),
		"GOBLIN_BIN="+filepath.Join(binDir, "goblin"),
		"GOBLIN_GUARD="+filepath.Join(binDir, "gguard"),
		"XDG_RUNTIME_DIR="+runtimeDir,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gash: %v\n%s", err, out)
	} else {
		t.Logf("gash output: %s", out)
	}

	waitFile(t, marker, "worked")
	waitFile(t, hook, "done")

	// El goblin se retiró con el script: no debe quedar manifiesto.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(runtimeDir, "goblin", name+".json")); os.IsNotExist(err) {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("el manifiesto de gash-test quedó huérfano")
}

func TestParseViaAPI(t *testing.T) {
	s, err := gash.Parse("grun x\n  echo hola\nnurg\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 1 || s.Items[0].Step == nil || s.Items[0].Step.Def != "x" {
		t.Fatalf("items=%+v", s.Items)
	}
}

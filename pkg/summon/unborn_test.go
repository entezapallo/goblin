package summon_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goblin/pkg/summon"
)

// Fuerza un fallo de nacimiento (binario goblin inexistente) y verifica
// on_unborn: kill limpia el cgroup, keep lo deja, y siempre hay log.
func TestUnbornPolicy(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere cgroupfs real")
	}
	gg, _, guard := buildTestBinaries(t)
	out, err := exec.Command(gg, "discover").Output()
	if err != nil {
		t.Fatal(err)
	}
	ancestor := strings.TrimSpace(string(out))

	for _, policy := range []string{"kill", "keep"} {
		t.Run(policy, func(t *testing.T) {
			runtimeDir := t.TempDir()
			id := fmt.Sprintf("unborn-%s-%d-%d", policy, os.Getpid(), time.Now().UnixNano())
			_, err := summon.StartSession(summon.Config{
				ID: id, GGPath: gg,
				GoblinBin: filepath.Join(runtimeDir, "no-existe"),
				GuardBin:  guard, RuntimeDir: runtimeDir,
				UnbornPolicy: policy,
			})
			if err == nil {
				t.Fatal("se esperaba fallo de nacimiento")
			}

			// Siempre hay notificación de unborn (el "log" del default).
			data, err := os.ReadFile(filepath.Join(runtimeDir, "goblin", "notifications.jsonl"))
			if err != nil || !strings.Contains(string(data), `"channel":"unborn"`) {
				t.Fatalf("notifications=%q err=%v", data, err)
			}

			cgroup := filepath.Join(ancestor, "goblin", id)
			_, statErr := os.Stat(cgroup)
			switch policy {
			case "kill":
				if !os.IsNotExist(statErr) {
					t.Fatalf("kill dejó el cgroup: %v", statErr)
				}
			case "keep":
				if statErr != nil {
					t.Fatalf("keep borró el cgroup: %v", statErr)
				}
				summon.CleanupByName(gg, ancestor, id)
			}
		})
	}
}

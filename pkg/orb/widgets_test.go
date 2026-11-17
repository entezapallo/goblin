package orb_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"goblin/pkg/orb"
)

func TestConfigDefaults(t *testing.T) {
	cfg := orb.DefaultConfig()
	if len(cfg.Widgets) != 4 || cfg.Heights["goblins"] <= 0 {
		t.Fatalf("defaults=%+v", cfg)
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "orb", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data := `{"widgets":["top","events"],"heights":{"top":2,"events":8}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := orb.LoadConfig()
	if len(cfg.Widgets) != 2 || cfg.Widgets[1] != "events" || cfg.Heights["events"] != 8 {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestLoadConfigInvalidFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "orb", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{mal json"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := orb.LoadConfig()
	if len(cfg.Widgets) != 4 {
		t.Fatalf("defaults no aplicados: %+v", cfg)
	}
}

func TestEventsOrdered(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	sub := filepath.Join(dir, "goblin")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}
	old := `{"ts":"2020-01-01T00:00:00Z","channel":"old","data":"primero"}`
	new := `{"ts":"2024-01-01T00:00:00Z","channel":"new","data":"segundo"}`
	if err := os.WriteFile(filepath.Join(sub, "notifications.jsonl"), []byte(old+"\n"+new+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	events := orb.Events("", 5)
	if len(events) != 2 {
		t.Fatalf("events=%d", len(events))
	}
	if !events[0].Time.After(events[1].Time) {
		t.Fatalf("no ordenados: %+v", events)
	}
	if events[0].Text == "" {
		t.Fatal("texto vacío")
	}
}

func TestSparklines(t *testing.T) {
	s := orb.NewSampler(4)
	// Sin muestras, devuelve espacios.
	cpu, _ := s.Sparklines()
	if len(cpu) == 0 {
		t.Fatal("sparkline vacío")
	}
	// Captura sobre un goblin sin cgroup real: valores en cero, sin panic.
	s.Capture([]orb.Goblin{{ID: "x", CGroup: filepath.Join(t.TempDir(), "none")}})
	cpu, mem := s.Sparklines()
	if len(cpu) != 1 || len(mem) != 1 {
		t.Fatalf("sparklines cpu=%q mem=%q", cpu, mem)
	}
	// El window se respeta.
	for i := 0; i < 10; i++ {
		s.Capture(nil)
	}
	cpu, _ = s.Sparklines()
	if len(cpu) > 4 {
		t.Fatalf("window no respetado: %q", cpu)
	}
}

func TestEventTimeParsing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	sub := filepath.Join(dir, "goblin")
	_ = os.MkdirAll(sub, 0700)
	_ = os.WriteFile(filepath.Join(sub, "history.jsonl"), []byte(
		`{"goblin_pid":123,"worker_pid":130,"cmd":["echo","hi"],"state":"success","ended_at":"2024-06-01T12:00:00Z"}`+"\n"), 0600)
	events := orb.Events("", 2)
	if len(events) != 1 || events[0].Time.IsZero() {
		t.Fatalf("events=%+v", events)
	}
	_ = time.Second
}

package notify_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"goblin/pkg/notify"
)

func TestEmit(t *testing.T) {
	dir := t.TempDir()
	if err := notify.Emit(dir, "email", "hola"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "goblin", "notifications.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var got notify.Event
	if err := json.Unmarshal(bytes.TrimSpace(data), &got); err != nil {
		t.Fatal(err)
	}
	if got.Channel != "email" || got.Data != "hola" || got.TS.IsZero() {
		t.Fatalf("event=%+v", got)
	}
}

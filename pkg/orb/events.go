package orb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Event es una línea ya interpretada de history.jsonl o notifications.jsonl.
type Event struct {
	Time time.Time
	Text string
}

// Events devuelve las últimas n líneas de history y notifications,
// ordenadas por timestamp (más recientes primero).
func Events(runtimeDir string, n int) []Event {
	if runtimeDir == "" {
		runtimeDir = os.Getenv("XDG_RUNTIME_DIR")
	}
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	dir := filepath.Join(runtimeDir, "goblin")
	var out []Event
	for _, name := range []string{"history.jsonl", "notifications.jsonl"} {
		out = append(out, readEventFile(filepath.Join(dir, name))...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func readEventFile(path string) []Event {
	f, err := os.Open(path) // #nosec G304
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		ts := time.Time{}
		if json.Unmarshal([]byte(line), &obj) == nil {
			for _, key := range []string{"ts", "ended_at", "started_at", "time"} {
				if s, ok := obj[key].(string); ok {
					if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
						ts = t
						break
					}
				}
			}
		}
		out = append(out, Event{Time: ts, Text: formatEvent(obj, line)})
	}
	return out
}

// formatEvent produce una línea corta para la pantalla.
func formatEvent(obj map[string]any, raw string) string {
	if obj == nil {
		return truncate(raw, 100)
	}
	if _, ok := obj["goblin_pid"]; ok {
		return fmt.Sprintf("goblin %v  %v  %v", obj["goblin_pid"], obj["state"], summary(obj["cmd"]))
	}
	if ch, ok := obj["channel"]; ok {
		return fmt.Sprintf("notify %v  %v", ch, obj["data"])
	}
	return truncate(raw, 100)
}

func summary(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return truncate(s, 40)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return truncate(string(b), 40)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

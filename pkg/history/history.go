// Package history registra el historial de jobs de goblin en dos
// formas: un log detallado por goblin (vive y muere con él, útil para
// debug de ese script en particular) y un resumen global de jobs
// terminados (persiste entre goblins, con retención de 1 mes).
package history

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const retentionPeriod = 30 * 24 * time.Hour

// --- Log por goblin -------------------------------------------------

func GoblinLogPath(pid int) string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return filepath.Join(runtimeDir, "goblin", strconv.Itoa(pid)+".history.jsonl")
}

// AppendGoblinLog agrega una línea al log detallado del goblin pid.
// O_APPEND garantiza que la escritura no se entrelaza con otras,
// siempre que la línea entre en un solo write() del kernel — de sobra
// para una línea JSON de este tamaño.
func AppendGoblinLog(pid int, evt any) error {
	return appendLine(GoblinLogPath(pid), evt)
}

// --- Resumen global ---------------------------------------------------

// SummaryEntry es una línea del resumen global: un job que terminó.
type SummaryEntry struct {
	GoblinPID int       `json:"goblin_pid"`
	WorkerPID int       `json:"worker_pid"`
	Cmd       []string  `json:"cmd"`
	State     string    `json:"state"` // "done", "failed", "killed"
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

// SummaryPath devuelve dónde vive el resumen global, independiente de
// cualquier goblin individual.
func SummaryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".local", "share", "goblin", "history.jsonl")
}

// AppendSummary agrega entry al resumen global.
func AppendSummary(entry SummaryEntry) error {
	return appendLine(SummaryPath(), entry)
}

// Compact reescribe el resumen global descartando entradas más viejas
// que retentionPeriod (1 mes). No falla si el archivo no existe todavía.
// Usa un archivo temporal + rename atómico: si el proceso muere a mitad
// de la compactación, el archivo original queda intacto.
func Compact() error {
	path := SummaryPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-retentionPeriod)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".history-*.jsonl")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op si el rename ya lo movió

	dec := json.NewDecoder(bytes.NewReader(data))
	enc := json.NewEncoder(tmp)
	for {
		var entry SummaryEntry
		if err := dec.Decode(&entry); err != nil {
			break // EOF o línea corrupta: se corta ahí, se descarta el resto corrupto
		}
		if entry.EndedAt.After(cutoff) {
			if err := enc.Encode(entry); err != nil {
				tmp.Close()
				return err
			}
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// --- helpers ----------------------------------------------------------

func appendLine(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f) // Encode ya agrega \n al final de cada línea
	return enc.Encode(v)
}

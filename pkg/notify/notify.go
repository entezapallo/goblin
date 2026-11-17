// Package notify es el canal de salida de notify_when: un archivo JSONL
// local, observable con cat, sin red ni credenciales. No es un daemon.
package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Event es una línea de notifications.jsonl.
type Event struct {
	TS      time.Time `json:"ts"`
	Channel string    `json:"channel"`
	Data    string    `json:"data"`
}

// Dir devuelve el directorio de estado del usuario.
func Dir() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return filepath.Join(runtimeDir, "goblin")
}

// Path devuelve el archivo de notificaciones.
func Path(runtimeDir string) string {
	if runtimeDir == "" {
		return filepath.Join(Dir(), "notifications.jsonl")
	}
	return filepath.Join(runtimeDir, "goblin", "notifications.jsonl")
}

// Emit escribe una notificación. runtimeDir vacío usa XDG_RUNTIME_DIR.
func Emit(runtimeDir, channel, data string) error {
	path := Path(runtimeDir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // #nosec G304 -- path derivado del runtime dir
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(Event{TS: time.Now(), Channel: channel, Data: data})
}

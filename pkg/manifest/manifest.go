// Package manifest es el directorio de goblins vivos. Cada goblin escribe
// su placa al nacer y la borra al morir; no hay daemon persistente que
// posea el estado. Es la fuente de verdad para direccionar los verbos.
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manifest describe un goblin vivo.
type Manifest struct {
	ID        string    `json:"id"`
	PID       int       `json:"pid"`
	Starttime uint64    `json:"starttime,omitempty"`
	CGroup    string    `json:"cgroup"`
	Socket    string    `json:"socket"`
	OwnerPID  int       `json:"owner_pid"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

// Dir devuelve el directorio de placas, respetando XDG_RUNTIME_DIR.
func Dir() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return filepath.Join(runtimeDir, "goblin")
}

// Path devuelve la placa de un id.
func Path(id string) string {
	return filepath.Join(Dir(), id+".json")
}

// Write escribe la placa de forma atómica (temp + rename), para que un
// lector nunca vea un JSON a medias.
func Write(m *Manifest) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".manifest-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, Path(m.ID))
}

// Read lee la placa de un id.
func Read(id string) (*Manifest, error) {
	data, err := os.ReadFile(Path(id)) // #nosec G304 -- path derivado de id validado por el llamador
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// List devuelve todas las placas legibles. Las corruptas se ignoran: una
// placa rota no debe impedir ver los demás goblins.
func List() ([]*Manifest, error) {
	entries, err := os.ReadDir(Dir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Manifest
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(Dir(), name)) // #nosec G304 -- directorio controlado
		if err != nil {
			continue
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil || m.ID == "" {
			continue
		}
		out = append(out, &m)
	}
	return out, nil
}

// Remove borra la placa de un id. No falla si ya no existe.
func Remove(id string) error {
	err := os.Remove(Path(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

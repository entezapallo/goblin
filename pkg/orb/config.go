package orb

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config define qué widgets muestra orb y su altura relativa. El layout es
// configurable por archivo, no dinámico: se edita y se reinicia.
type Config struct {
	Widgets []string       `json:"widgets"`
	Heights map[string]int `json:"heights"`
}

// DefaultConfig es el layout esencial: cabecera con gráficos, dos tablas y
// el panel de eventos.
func DefaultConfig() Config {
	return Config{
		Widgets: []string{"top", "goblins", "workers", "events"},
		Heights: map[string]int{"top": 3, "goblins": 6, "workers": 6, "events": 5},
	}
}

// LoadConfig lee $XDG_CONFIG_HOME/orb/config.json. Si no existe o está mal,
// devuelve los defaults sin error: la TUI siempre debe poder arrancar.
func LoadConfig() Config {
	cfg := DefaultConfig()
	path := configPath()
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return cfg
	}
	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		return cfg
	}
	if len(loaded.Widgets) > 0 {
		cfg.Widgets = loaded.Widgets
	}
	for k, v := range loaded.Heights {
		if v > 0 {
			cfg.Heights[k] = v
		}
	}
	return cfg
}

func configPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "orb", "config.json")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "orb", "config.json")
}

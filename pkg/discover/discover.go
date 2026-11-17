// Package discover enumera los goblins vivos leyendo sus manifiestos, no
// sockets calculados por PPID. Es la capa de direccionamiento de los verbos:
// -g <ref> → GOBLIN_ID → ancestría por /proc → error con candidatos.
package discover

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"goblin/pkg/applets/gprocutil"
	"goblin/pkg/manifest"
)

// Goblin es la vista pública de un manifiesto.
type Goblin struct {
	ID         string
	PID        int
	OwnerPID   int
	CGroup     string
	SocketPath string
	State      string
}

func fromManifest(m *manifest.Manifest) Goblin {
	return Goblin{
		ID:         m.ID,
		PID:        m.PID,
		OwnerPID:   m.OwnerPID,
		CGroup:     m.CGroup,
		SocketPath: m.Socket,
		State:      m.State,
	}
}

func alive(m *manifest.Manifest) bool {
	if m.CGroup == "" {
		return false
	}
	_, err := os.Stat(m.CGroup)
	return err == nil
}

// Discover devuelve los goblins vivos. Las placas cuyo cgroup ya no existe
// se consideran huérfanas y se borran de paso.
func Discover() ([]Goblin, error) {
	ms, err := manifest.List()
	if err != nil {
		return nil, err
	}
	var out []Goblin
	for _, m := range ms {
		if !alive(m) {
			_ = manifest.Remove(m.ID)
			continue
		}
		out = append(out, fromManifest(m))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func candidates() string {
	goblins, _ := Discover()
	if len(goblins) == 0 {
		return "(ninguno)"
	}
	ids := make([]string, 0, len(goblins))
	for _, g := range goblins {
		ids = append(ids, g.ID)
	}
	return strings.Join(ids, ", ")
}

// Resolve busca un goblin por referencia explícita, variable de entorno o
// ancestría del proceso que llama. Es la política compartida por los verbos.
func Resolve(ref string) (*Goblin, error) {
	goblins, err := Discover()
	if err != nil {
		return nil, err
	}

	if ref != "" {
		for i := range goblins {
			if goblins[i].ID == ref {
				return &goblins[i], nil
			}
		}
		var matches []int
		for i := range goblins {
			if strings.HasPrefix(goblins[i].ID, ref) {
				matches = append(matches, i)
			}
		}
		switch len(matches) {
		case 0:
			return nil, fmt.Errorf("goblin no encontrado: %s (candidatos: %s)", ref, candidates())
		case 1:
			return &goblins[matches[0]], nil
		default:
			ids := make([]string, 0, len(matches))
			for _, i := range matches {
				ids = append(ids, goblins[i].ID)
			}
			return nil, fmt.Errorf("ref ambigua %s: %s", ref, strings.Join(ids, ", "))
		}
	}

	if envID := os.Getenv("GOBLIN_ID"); envID != "" {
		for i := range goblins {
			if goblins[i].ID == envID {
				return &goblins[i], nil
			}
		}
	}

	// Ancestría: subir por PPID real buscando el proceso goblin o el
	// instructor (owner) de algún manifiesto.
	for pid := os.Getpid(); pid > 1; {
		for i := range goblins {
			if goblins[i].PID == pid || goblins[i].OwnerPID == pid {
				return &goblins[i], nil
			}
		}
		ppid, _, _, _, _, _, _, _ := gprocutil.ReadStat(pid)
		if ppid <= 0 || ppid >= pid {
			break
		}
		pid = ppid
	}

	return nil, fmt.Errorf("no se pudo resolver el goblin (candidatos: %s)", candidates())
}

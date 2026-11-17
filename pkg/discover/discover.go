// Package discover enumera los goblins vivos del usuario actual,
// buscando sockets en $XDG_RUNTIME_DIR/goblin/. Reemplaza el fallback
// de gutils.SocketPath() (un solo path calculado por PPID) por
// descubrimiento real multi-goblin, necesario para el TUI y para
// listar/inspeccionar más de un goblin a la vez.
package discover

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// dialTimeout es cuánto se espera antes de considerar un socket muerto.
// Corto a propósito: es un socket local, si no responde ya casi seguro
// no hay nadie del otro lado.
const dialTimeout = 200 * time.Millisecond

// Goblin describe un goblin vivo encontrado por Discover.
type Goblin struct {
	PID        int
	SocketPath string
}

// baseDir devuelve $XDG_RUNTIME_DIR/goblin, con el mismo fallback a
// os.TempDir() que ya usa gutils.SocketPath().
func baseDir() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return filepath.Join(runtimeDir, "goblin")
}

// SocketPath devuelve el path que le corresponde a un homunculi con el
// PID dado. homunculi lo usa para saber dónde escuchar; Discover lo
// reconstruye al enumerar el directorio.
func SocketPath(pid int) string {
	return filepath.Join(baseDir(), strconv.Itoa(pid)+".sock")
}

// Discover enumera $XDG_RUNTIME_DIR/goblin/, se conecta a cada socket
// encontrado para confirmar que sigue vivo, y devuelve solo los que
// respondieron. Los sockets que no responden (proceso muerto sin
// limpieza) se borran de paso.
func Discover() ([]Goblin, error) {
	dir := baseDir()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // sin goblins corriendo, no es un error
	}
	if err != nil {
		return nil, err
	}

	var goblins []Goblin
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".sock") {
			continue
		}
		pidStr := strings.TrimSuffix(name, ".sock")
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue // no es un socket nuestro, nombre inesperado
		}

		path := filepath.Join(dir, name)
		conn, err := net.DialTimeout("unix", path, dialTimeout)
		if err != nil {
			os.Remove(path) // huérfano: nadie escuchando, limpieza gratis
			continue
		}
		conn.Close()

		goblins = append(goblins, Goblin{PID: pid, SocketPath: path})
	}

	return goblins, nil
}

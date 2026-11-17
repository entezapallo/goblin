// pkg/gutils/spawn.go
//
// Spawn es el mecanismo de lanzamiento: fork/exec real de un job, en su
// propio grupo de procesos (necesario para poder señalizar/matar el
// grupo entero, no solo el líder). No vigila el proceso después de
// lanzarlo — eso es responsabilidad de quien consuma el estado (el TUI,
// vía list/status), no de gutils.
package gutils

import (
	"os/exec"
	"syscall"
)

// Spawn lanza cmd[0] con cmd[1:] como argumentos, en su propio grupo de
// procesos, y devuelve el *exec.Cmd ya arrancado (Start(), no Run() —
// no bloquea esperando a que termine).
func Spawn(cmd []string) (*exec.Cmd, error) {
	c := exec.Command(cmd[0], cmd[1:]...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := c.Start(); err != nil {
		return nil, err
	}
	return c, nil
}

//go:build !js && !wasm && !wasip1

package startstopdaemon

import (
	"io"
	"os/exec"
	"syscall"
)

// SpawnConfig separa lo que puede pedirse por CLI de lo que no: CgroupFD
// y Pdeathsig solo se pueden pedir desde código, con esta firma tipada.
type SpawnConfig struct {
	Binary   string
	Args     []string
	CgroupFD int
	Env      []string  // si es nil, hereda os.Environ()
	Stdout   io.Writer // nil = descartar
	Stderr   io.Writer // nil = descartar
}

// SpawnInner lanza un proceso dentro del cgroup cuyo fd se recibe, con
// Pdeathsig=SIGKILL para que muera si muere el hilo que lo creó. Devuelve
// el *exec.Cmd para poder Wait() y hacer reap. Es el único pedazo
// compilado justificado: el shell no puede hacer esto.
func SpawnInner(cfg SpawnConfig) (*exec.Cmd, error) {
	cmd := exec.Command(cfg.Binary, cfg.Args...) // #nosec G204 -- comando pedido por el instructor
	cmd.SysProcAttr = &syscall.SysProcAttr{
		UseCgroupFD: true,
		CgroupFD:    cfg.CgroupFD,
		Pdeathsig:   syscall.SIGKILL,
	}
	if cfg.Env != nil {
		cmd.Env = cfg.Env
	}
	cmd.Stdout = cfg.Stdout
	cmd.Stderr = cfg.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

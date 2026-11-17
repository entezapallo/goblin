package startstopdaemon

import (
	"io"
	"os"
	"os/exec"
	"syscall"
)

// SpawnConfig separa lo que puede pedirse por CLI de lo que no: CgroupFD,
// ExtraFiles y el resto solo se pueden pedir desde código, con esta firma
// tipada (no hay flag que los exponga).
type SpawnConfig struct {
	Binary     string
	Args       []string
	CgroupFD   int
	Env        []string   // si es nil, hereda os.Environ()
	Stdout     io.Writer  // nil = descartar
	Stderr     io.Writer  // nil = descartar
	ExtraFiles []*os.File // fd 3, 4... en el hijo
}

// SpawnInner lanza un proceso dentro del cgroup cuyo fd se recibe y devuelve
// el *exec.Cmd para poder Wait()/reap. No se usa Pdeathsig: con el runtime de
// Go mata al worker cuando muere el HILO que lo creó, no el proceso. La
// garantía de muerte la da gguard (pipe de liveness) + cgroup.kill.
func SpawnInner(cfg SpawnConfig) (*exec.Cmd, error) {
	cmd := exec.Command(cfg.Binary, cfg.Args...) // #nosec G204 -- comando pedido por el instructor
	cmd.SysProcAttr = &syscall.SysProcAttr{
		UseCgroupFD: true,
		CgroupFD:    cfg.CgroupFD,
	}
	if cfg.Env != nil {
		cmd.Env = cfg.Env
	}
	cmd.Stdout = cfg.Stdout
	cmd.Stderr = cfg.Stderr
	cmd.ExtraFiles = cfg.ExtraFiles
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

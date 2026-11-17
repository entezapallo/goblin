// pkg/gutils/spawn.go
//
// SpawnInto lanza un worker dentro de su cgroup, detrás de gguard. El fd del
// cgroup se pasa a startstopdaemon.SpawnInner (UseCgroupFD) y el fd de
// liveness al guard por ExtraFiles (fd 3). gutils no reimplementa
// exec.Command. Las salidas se entregan al actor para las reglas de texto.
package gutils

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"goblin/pkg/applets/startstopdaemon"
)

// SpawnInto lanza guardBin "3" -- cmd... dentro de cgroupPath, sin esperar a
// que termine, y devuelve el *exec.Cmd del guard. El hijo del guard es el
// worker real y hereda GOBLIN_ID para direccionar a su goblin.
func SpawnInto(id, cgroupPath, workDir string, argv []string, stdout, stderr io.Writer, livenessRead *os.File, guardBin string) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("goblin: comando vacío")
	}
	if guardBin == "" {
		guardBin = "gguard"
	}
	dir, err := os.Open(cgroupPath) // #nosec G304 -- cgroup path viene del manifiesto
	if err != nil {
		return nil, fmt.Errorf("goblin: abrir cgroup %s: %w", cgroupPath, err)
	}
	defer dir.Close()

	env := append(os.Environ(), "GOBLIN_ID="+id, "GOBLIN_WORKDIR="+workDir)
	guardArgs := append([]string{"3", "--"}, argv...)
	return startstopdaemon.SpawnInner(startstopdaemon.SpawnConfig{
		Binary:     guardBin,
		Args:       guardArgs,
		CgroupFD:   int(dir.Fd()),
		Env:        env,
		Stdout:     stdout,
		Stderr:     stderr,
		ExtraFiles: []*os.File{livenessRead},
	})
}

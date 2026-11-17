// pkg/gutils/spawn.go
//
// SpawnInto es el lanzamiento de un worker dentro de su cgroup. El fd del
// cgroup se pasa a startstopdaemon.SpawnInner (UseCgroupFD + Pdeathsig);
// gutils no reimplementa exec.Command. Las salidas se entregan al actor
// para alimentar las reglas de texto sobre stdout/stderr.
package gutils

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"goblin/pkg/applets/startstopdaemon"
)

// SpawnInto lanza argv[0] con argv[1:] dentro de cgroupPath, en-start sin
// esperar a que termine, y devuelve el *os.Process ya arrancado. El worker
// hereda GOBLIN_ID para poder direccionar al goblin que lo creó.
func SpawnInto(id, cgroupPath string, argv []string, stdout, stderr io.Writer) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("goblin: comando vacío")
	}
	dir, err := os.Open(cgroupPath) // #nosec G304 -- cgroup path viene del manifiesto
	if err != nil {
		return nil, fmt.Errorf("goblin: abrir cgroup %s: %w", cgroupPath, err)
	}
	defer dir.Close()

	env := append(os.Environ(), "GOBLIN_ID="+id)
	return startstopdaemon.SpawnInner(startstopdaemon.SpawnConfig{
		Binary:   argv[0],
		Args:     argv[1:],
		CgroupFD: int(dir.Fd()),
		Env:      env,
		Stdout:   stdout,
		Stderr:   stderr,
	})
}

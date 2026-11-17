// Package verbs contiene la lógica compartida por los applets que mutan
// un goblin: resolver el target y mandar el evento por su socket efímero.
// Las consultas (list/stat) NO pasan por acá: leen cgroupfs/manifiestos.
package verbs

import (
	"fmt"

	"goblin/pkg/discover"
	"goblin/pkg/gutils"
)

// ParseRef extrae -g <ref> del argv. Devuelve la ref (puede ser "") y el
// resto de los argumentos.
func ParseRef(args []string) (string, []string) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-g" || args[i] == "--goblin" {
			ref := args[i+1]
			rest := append([]string{}, args[:i]...)
			return ref, append(rest, args[i+2:]...)
		}
	}
	return "", args
}

// Mutate resuelve el goblin y le envía una mutación por NDJSON.
func Mutate(ref, verb string, pid int, cmd []string) (gutils.Event, error) {
	g, err := discover.Resolve(ref)
	if err != nil {
		return gutils.Event{}, err
	}
	if g.SocketPath == "" {
		return gutils.Event{}, fmt.Errorf("goblin %s sin socket", g.ID)
	}
	return gutils.Call(g.SocketPath, gutils.Event{Type: verb, PID: pid, Cmd: cmd})
}

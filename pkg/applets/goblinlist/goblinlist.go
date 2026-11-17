// Package goblinlist es la consulta de goblins vivos. No habla por socket:
// lee los manifiestos y verifica que el cgroup siga existiendo (fase
// híbrida: consultas a cgroupfs, mutaciones por socket).
package goblinlist

import (
	"fmt"

	"goblin/pkg/core"
	"goblin/pkg/discover"
)

// Run ejecuta goblinlist con los argumentos dados.
//
// Uso:
//
//	goblinlist
//
// Imprime una línea por goblin: ID PID STATE CGROUP SOCKET.
func Run(stdio *core.Stdio, args []string) int {
	goblins, err := discover.Discover()
	if err != nil {
		stdio.Errorf("goblinlist: %v\n", err)
		return core.ExitFailure
	}
	for _, g := range goblins {
		fmt.Fprintf(stdio.Out, "%s\t%d\t%s\t%s\t%s\n", g.ID, g.PID, g.State, g.CGroup, g.SocketPath)
	}
	return core.ExitSuccess
}

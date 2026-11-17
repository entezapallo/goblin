// Package goblinsubmit es el cliente NDJSON delgado para el verbo
// "submit": le pide al goblin corriendo que lance un nuevo job.
package goblinsubmit

import (
	"goblin/pkg/core"
	"goblin/pkg/gutils"
)

// Run ejecuta goblinsubmit con los argumentos dados.
//
// Uso:
//
//	goblinsubmit CMD [ARGS...]
func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 1 {
		return core.UsageError(stdio, "goblinsubmit", "falta comando a lanzar")
	}

	sock := gutils.SocketPath()

	resp, err := gutils.Call(sock, gutils.Event{
		Type: "submit",
		Cmd:  args,
	})
	if err != nil {
		stdio.Errorf("goblinsubmit: %v\n", err)
		return core.ExitFailure
	}

	stdio.Println(resp)
	return core.ExitSuccess
}

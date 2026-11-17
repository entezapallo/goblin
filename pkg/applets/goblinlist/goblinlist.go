// Package goblinlist es el cliente NDJSON delgado para el verbo "list":
// pide al goblin corriendo (vía gutils.Call) la lista de jobs que
// gestiona, y la imprime. Mismo patrón Run(stdio,args) que el resto
// del árbol.
package goblinlist

import (
	"errors"

	"goblin/pkg/core"
	"goblin/pkg/gutils"
)

// Run ejecuta goblinlist con los argumentos dados.
//
// Uso:
//
//	goblinlist
func Run(stdio *core.Stdio, args []string) int {
	sock := gutils.SocketPath()
	if sock == "" {
		return core.FileError(stdio, "goblinlist", "socket", errors.New("no se pudo resolver el socket del goblin"))
	}

	resp, err := gutils.SimpleVerb(sock, "list", 0)
	if err != nil {
		stdio.Errorf("goblinlist: %v\n", err)
		return core.ExitFailure
	}

	stdio.Println(resp)
	return core.ExitSuccess
}

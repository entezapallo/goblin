// Package goblinpause es el cliente NDJSON delgado para el verbo "pause":
// le pide al goblin corriendo que pause el job con el PID dado.
package goblinpause

import (
	"errors"
	"strconv"

	"goblin/pkg/core"
	"goblin/pkg/gutils"
)

// Run ejecuta goblinpause con los argumentos dados.
//
// Uso:
//
//	goblinpause PID
func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 1 {
		return core.UsageError(stdio, "goblinpause", "falta PID")
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil {
		return core.UsageError(stdio, "goblinpause", "PID inválido: "+args[0])
	}

	sock := gutils.SocketPath()
	if sock == "" {
		return core.FileError(stdio, "goblinpause", "socket", errors.New("no se pudo resolver el socket del goblin"))
	}

	resp, err := gutils.SimpleVerb(sock, "pause", pid)
	if err != nil {
		stdio.Errorf("goblinpause: %v\n", err)
		return core.ExitFailure
	}

	stdio.Println(resp)
	return core.ExitSuccess
}

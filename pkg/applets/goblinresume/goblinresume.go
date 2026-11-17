// Package goblinresume es el cliente NDJSON delgado para el verbo
// "resume": le pide al goblin corriendo que reanude el job con el PID dado.
package goblinresume

import (
	"errors"
	"strconv"

	"goblin/pkg/core"
	"goblin/pkg/gutils"
)

// Run ejecuta goblinresume con los argumentos dados.
//
// Uso:
//
//	goblinresume PID
func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 1 {
		return core.UsageError(stdio, "goblinresume", "falta PID")
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil {
		return core.UsageError(stdio, "goblinresume", "PID inválido: "+args[0])
	}

	sock := gutils.SocketPath()
	if sock == "" {
		return core.FileError(stdio, "goblinresume", "socket", errors.New("no se pudo resolver el socket del goblin"))
	}

	resp, err := gutils.SimpleVerb(sock, "resume", pid)
	if err != nil {
		stdio.Errorf("goblinresume: %v\n", err)
		return core.ExitFailure
	}

	stdio.Println(resp)
	return core.ExitSuccess
}

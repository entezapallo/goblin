// Package goblinkill es el cliente NDJSON delgado para el verbo "kill":
// le pide al goblin corriendo que mate el job con el PID dado (y, por
// la garantía dura del sistema, todo lo que gestiona con él).
package goblinkill

import (
	"errors"
	"strconv"

	"goblin/pkg/core"
	"goblin/pkg/gutils"
)

// Run ejecuta goblinkill con los argumentos dados.
//
// Uso:
//
//	goblinkill PID
func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 1 {
		return core.UsageError(stdio, "goblinkill", "falta PID")
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil {
		return core.UsageError(stdio, "goblinkill", "PID inválido: "+args[0])
	}

	sock := gutils.SocketPath()
	if sock == "" {
		return core.FileError(stdio, "goblinkill", "socket", errors.New("no se pudo resolver el socket del goblin"))
	}

	resp, err := gutils.SimpleVerb(sock, "kill", pid)
	if err != nil {
		stdio.Errorf("goblinkill: %v\n", err)
		return core.ExitFailure
	}

	stdio.Println(resp)
	return core.ExitSuccess
}

// Package goblinkill es el cliente del verbo "kill": pide al goblin que
// mate el worker con el PID dado (y todo su subárbol).
package goblinkill

import (
	"strconv"

	"goblin/pkg/core"
	"goblin/pkg/verbs"
)

// Run ejecuta goblinkill con los argumentos dados.
//
// Uso:
//
//	goblinkill [-g ID] PID
func Run(stdio *core.Stdio, args []string) int {
	ref, rest := verbs.ParseRef(args)
	if len(rest) < 1 {
		return core.UsageError(stdio, "goblinkill", "falta PID")
	}
	pid, err := strconv.Atoi(rest[0])
	if err != nil {
		return core.UsageError(stdio, "goblinkill", "PID inválido: "+rest[0])
	}

	resp, err := verbs.Mutate(ref, "kill", pid, nil)
	if err != nil {
		stdio.Errorf("goblinkill: %v\n", err)
		return core.ExitFailure
	}
	stdio.Println(resp)
	return core.ExitSuccess
}

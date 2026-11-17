// Package goblinresume es el cliente del verbo "resume": pide al goblin que
// descongele el worker con el PID dado.
package goblinresume

import (
	"strconv"

	"goblin/pkg/core"
	"goblin/pkg/verbs"
)

// Run ejecuta goblinresume con los argumentos dados.
//
// Uso:
//
//	goblinresume [-g ID] PID
func Run(stdio *core.Stdio, args []string) int {
	ref, rest := verbs.ParseRef(args)
	if len(rest) < 1 {
		return core.UsageError(stdio, "goblinresume", "falta PID")
	}
	pid, err := strconv.Atoi(rest[0])
	if err != nil {
		return core.UsageError(stdio, "goblinresume", "PID inválido: "+rest[0])
	}

	resp, err := verbs.Mutate(ref, "resume", pid, nil)
	if err != nil {
		stdio.Errorf("goblinresume: %v\n", err)
		return core.ExitFailure
	}
	stdio.Println(resp)
	return core.ExitSuccess
}

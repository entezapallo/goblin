// Package goblinpause es el cliente del verbo "pause": pide al goblin que
// congele el worker con el PID dado (cgroup.freeze, pausa real).
package goblinpause

import (
	"strconv"

	"goblin/pkg/core"
	"goblin/pkg/verbs"
)

// Run ejecuta goblinpause con los argumentos dados.
//
// Uso:
//
//	goblinpause [-g ID] PID
func Run(stdio *core.Stdio, args []string) int {
	ref, rest := verbs.ParseRef(args)
	if len(rest) < 1 {
		return core.UsageError(stdio, "goblinpause", "falta PID")
	}
	pid, err := strconv.Atoi(rest[0])
	if err != nil {
		return core.UsageError(stdio, "goblinpause", "PID inválido: "+rest[0])
	}

	resp, err := verbs.Mutate(ref, "pause", pid, nil)
	if err != nil {
		stdio.Errorf("goblinpause: %v\n", err)
		return core.ExitFailure
	}
	stdio.Println(resp)
	return core.ExitSuccess
}

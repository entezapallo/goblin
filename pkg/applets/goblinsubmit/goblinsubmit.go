// Package goblinsubmit es el cliente del verbo "submit": le pide al goblin
// (resuelto por -g / GOBLIN_ID / ancestría) que lance un job.
package goblinsubmit

import (
	"goblin/pkg/core"
	"goblin/pkg/verbs"
)

// Run ejecuta goblinsubmit con los argumentos dados.
//
// Uso:
//
//	goblinsubmit [-g ID] CMD [ARGS...]
func Run(stdio *core.Stdio, args []string) int {
	ref, rest := verbs.ParseRef(args)
	if len(rest) < 1 {
		return core.UsageError(stdio, "goblinsubmit", "falta comando a lanzar")
	}

	resp, err := verbs.Mutate(ref, "submit", 0, rest)
	if err != nil {
		stdio.Errorf("goblinsubmit: %v\n", err)
		return core.ExitFailure
	}
	stdio.Println(resp)
	return core.ExitSuccess
}

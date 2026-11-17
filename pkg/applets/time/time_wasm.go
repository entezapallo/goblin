//go:build js || wasm || wasip1

package time

import "goblin/pkg/core"

// Run is a stub that returns an error on WASM platforms where time is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("time: not supported in wasm\n")
	return core.ExitFailure
}

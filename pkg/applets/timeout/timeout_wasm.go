//go:build js || wasm || wasip1

package timeout

import "goblin/pkg/core"

// Run is a stub that returns an error on WASM platforms where timeout is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("timeout: not supported in wasm\n")
	return core.ExitFailure
}

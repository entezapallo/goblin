// Command cp is a standalone entry point for the cp applet.
package main

import (
	"os"

	"goblin/pkg/applets/cp"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(cp.Run(stdio, os.Args[1:]))
}

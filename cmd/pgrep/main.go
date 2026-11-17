// Command pgrep is a standalone entry point for the pgrep applet.
package main

import (
	"os"

	"goblin/pkg/applets/pgrep"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(pgrep.Run(stdio, os.Args[1:]))
}

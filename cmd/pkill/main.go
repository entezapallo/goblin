// Command pkill is a standalone entry point for the pkill applet.
package main

import (
	"os"

	"goblin/pkg/applets/pkill"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(pkill.Run(stdio, os.Args[1:]))
}

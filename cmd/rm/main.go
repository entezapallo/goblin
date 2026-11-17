// Command rm is a standalone entry point for the rm applet.
package main

import (
	"os"

	"goblin/pkg/applets/rm"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(rm.Run(stdio, os.Args[1:]))
}

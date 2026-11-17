// Command setsid is a standalone entry point for the setsid applet.
package main

import (
	"os"

	"goblin/pkg/applets/setsid"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(setsid.Run(stdio, os.Args[1:]))
}

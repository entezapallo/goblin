// Command startstopdaemon is a standalone entry point for the startstopdaemon applet.
package main

import (
	"os"

	"goblin/pkg/applets/startstopdaemon"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(startstopdaemon.Run(stdio, os.Args[1:]))
}

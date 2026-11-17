// Command pidof is a standalone entry point for the pidof applet.
package main

import (
	"os"

	"goblin/pkg/applets/pidof"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(pidof.Run(stdio, os.Args[1:]))
}

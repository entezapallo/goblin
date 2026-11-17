// Command mv is a standalone entry point for the mv applet.
package main

import (
	"os"

	"goblin/pkg/applets/mv"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(mv.Run(stdio, os.Args[1:]))
}

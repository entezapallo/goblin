// Command cut is a standalone entry point for the cut applet.
package main

import (
	"os"

	"goblin/pkg/applets/cut"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(cut.Run(stdio, os.Args[1:]))
}

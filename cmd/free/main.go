// Command free is a standalone entry point for the free applet.
package main

import (
	"os"

	"goblin/pkg/applets/free"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(free.Run(stdio, os.Args[1:]))
}

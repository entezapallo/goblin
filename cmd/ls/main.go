// Command ls is a standalone entry point for the ls applet.
package main

import (
	"os"

	"goblin/pkg/applets/ls"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(ls.Run(stdio, os.Args[1:]))
}

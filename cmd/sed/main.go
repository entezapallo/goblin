// Command sed is a standalone entry point for the sed applet.
package main

import (
	"os"

	"goblin/pkg/applets/sed"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(sed.Run(stdio, os.Args[1:]))
}

// Command rmdir is a standalone entry point for the rmdir applet.
package main

import (
	"os"

	"goblin/pkg/applets/rmdir"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(rmdir.Run(stdio, os.Args[1:]))
}

// Command mkdir is a standalone entry point for the mkdir applet.
package main

import (
	"os"

	"goblin/pkg/applets/mkdir"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(mkdir.Run(stdio, os.Args[1:]))
}

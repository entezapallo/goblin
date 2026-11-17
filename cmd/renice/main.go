// Command renice is a standalone entry point for the renice applet.
package main

import (
	"os"

	"goblin/pkg/applets/renice"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(renice.Run(stdio, os.Args[1:]))
}

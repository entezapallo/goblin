// Command ps is a standalone entry point for the ps applet.
package main

import (
	"os"

	"goblin/pkg/applets/ps"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(ps.Run(stdio, os.Args[1:]))
}

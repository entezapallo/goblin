// Command gzip is a standalone entry point for the gzip applet.
package main

import (
	"os"

	"goblin/pkg/applets/gzip"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(gzip.Run(stdio, os.Args[1:]))
}

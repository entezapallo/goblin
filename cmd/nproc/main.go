// Command nproc is a standalone entry point for the nproc applet.
package main

import (
	"os"

	"goblin/pkg/applets/nproc"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(nproc.Run(stdio, os.Args[1:]))
}

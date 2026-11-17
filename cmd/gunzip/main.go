// Command gunzip is a standalone entry point for the gunzip applet.
package main

import (
	"os"

	"goblin/pkg/applets/gunzip"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(gunzip.Run(stdio, os.Args[1:]))
}

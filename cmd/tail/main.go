// Command tail is a standalone entry point for the tail applet.
package main

import (
	"os"

	"goblin/pkg/applets/tail"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(tail.Run(stdio, os.Args[1:]))
}

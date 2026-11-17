// Command timeout is a standalone entry point for the timeout applet.
package main

import (
	"os"

	"goblin/pkg/applets/timeout"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(timeout.Run(stdio, os.Args[1:]))
}

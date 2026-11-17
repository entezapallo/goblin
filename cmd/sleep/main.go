// Command sleep is a standalone entry point for the sleep applet.
package main

import (
	"os"

	"goblin/pkg/applets/sleep"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(sleep.Run(stdio, os.Args[1:]))
}

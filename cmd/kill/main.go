// Command kill is a standalone entry point for the kill applet.
package main

import (
	"os"

	"goblin/pkg/applets/kill"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(kill.Run(stdio, os.Args[1:]))
}

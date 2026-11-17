// Command nice is a standalone entry point for the nice applet.
package main

import (
	"os"

	"goblin/pkg/applets/nice"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(nice.Run(stdio, os.Args[1:]))
}

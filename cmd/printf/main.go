// Command printf is a standalone entry point for the printf applet.
package main

import (
	"os"

	"goblin/pkg/applets/printf"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(printf.Run(stdio, os.Args[1:]))
}

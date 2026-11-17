// Command find is a standalone entry point for the find applet.
package main

import (
	"os"

	"goblin/pkg/applets/find"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(find.Run(stdio, os.Args[1:]))
}

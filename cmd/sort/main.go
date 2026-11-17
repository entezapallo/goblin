// Command sort is a standalone entry point for the sort applet.
package main

import (
	"os"

	"goblin/pkg/applets/sort"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(sort.Run(stdio, os.Args[1:]))
}

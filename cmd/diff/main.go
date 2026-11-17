// Command diff is a standalone entry point for the diff applet.
package main

import (
	"os"

	"goblin/pkg/applets/diff"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(diff.Run(stdio, os.Args[1:]))
}

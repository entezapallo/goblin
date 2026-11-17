// Command awk is a standalone entry point for the awk applet.
package main

import (
	"os"

	"goblin/pkg/applets/awk"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(awk.Run(stdio, os.Args[1:]))
}

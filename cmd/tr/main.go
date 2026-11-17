// Command tr is a standalone entry point for the tr applet.
package main

import (
	"os"

	"goblin/pkg/applets/tr"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(tr.Run(stdio, os.Args[1:]))
}

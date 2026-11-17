// Command wc is a standalone entry point for the wc applet.
package main

import (
	"os"

	"goblin/pkg/applets/wc"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(wc.Run(stdio, os.Args[1:]))
}

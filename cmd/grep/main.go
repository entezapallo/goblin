// Command grep is a standalone entry point for the grep applet.
package main

import (
	"os"

	"goblin/pkg/applets/grep"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(grep.Run(stdio, os.Args[1:]))
}

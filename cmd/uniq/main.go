// Command uniq is a standalone entry point for the uniq applet.
package main

import (
	"os"

	"goblin/pkg/applets/uniq"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(uniq.Run(stdio, os.Args[1:]))
}

// Command cat is a standalone entry point for the cat applet.
package main

import (
	"os"

	"goblin/pkg/applets/cat"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(cat.Run(stdio, os.Args[1:]))
}

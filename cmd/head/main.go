// Command head is a standalone entry point for the head applet.
package main

import (
	"os"

	"goblin/pkg/applets/head"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(head.Run(stdio, os.Args[1:]))
}

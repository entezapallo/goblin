// Command ash is a standalone entry point for the ash applet.
package main

import (
	"os"

	"goblin/pkg/applets/ash"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(ash.Run(stdio, os.Args[1:]))
}

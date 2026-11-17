// Command time is a standalone entry point for the time applet.
package main

import (
	"os"

	"goblin/pkg/applets/time"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(time.Run(stdio, os.Args[1:]))
}

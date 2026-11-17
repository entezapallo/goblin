// Command uptime is a standalone entry point for the uptime applet.
package main

import (
	"os"

	"goblin/pkg/applets/uptime"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(uptime.Run(stdio, os.Args[1:]))
}

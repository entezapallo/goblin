// Command xargs is a standalone entry point for the xargs applet.
package main

import (
	"os"

	"goblin/pkg/applets/xargs"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(xargs.Run(stdio, os.Args[1:]))
}

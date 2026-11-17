// Command echo is a standalone entry point for the echo applet.
package main

import (
	"os"

	"goblin/pkg/applets/echo"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(echo.Run(stdio, os.Args[1:]))
}

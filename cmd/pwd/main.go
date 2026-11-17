// Command pwd is a standalone entry point for the pwd applet.
package main

import (
	"os"

	"goblin/pkg/applets/pwd"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(pwd.Run(stdio, os.Args[1:]))
}

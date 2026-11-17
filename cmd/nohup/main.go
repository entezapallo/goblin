// Command nohup is a standalone entry point for the nohup applet.
package main

import (
	"os"

	"goblin/pkg/applets/nohup"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(nohup.Run(stdio, os.Args[1:]))
}

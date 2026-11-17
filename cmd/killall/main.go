// Command killall is a standalone entry point for the killall applet.
package main

import (
	"os"

	"goblin/pkg/applets/killall"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(killall.Run(stdio, os.Args[1:]))
}

// Command taskset is a standalone entry point for the taskset applet.
package main

import (
	"os"

	"goblin/pkg/applets/taskset"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(taskset.Run(stdio, os.Args[1:]))
}

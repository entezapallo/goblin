// Command tar is a standalone entry point for the tar applet.
package main

import (
	"os"

	"goblin/pkg/applets/tar"
	"goblin/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(tar.Run(stdio, os.Args[1:]))
}

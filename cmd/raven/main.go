// cmd/raven/main.go
//
// raven es el emisor de notificaciones del contract: una CLI delgada sobre
// pkg/notify. No es un daemon y no habla por red: escribe en
// notifications.jsonl para que cualquier proceso pueda observar los avisos.
//
// Uso:
//
//	raven emit <channel> <data>
package main

import (
	"fmt"
	"os"

	"goblin/pkg/notify"
)

func main() {
	if len(os.Args) < 4 || os.Args[1] != "emit" {
		fmt.Fprintln(os.Stderr, "uso: raven emit <channel> <data>")
		os.Exit(2)
	}
	if err := notify.Emit("", os.Args[2], os.Args[3]); err != nil {
		fmt.Fprintf(os.Stderr, "raven: %v\n", err)
		os.Exit(1)
	}
}

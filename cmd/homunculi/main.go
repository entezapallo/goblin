// cmd/homunculi/main.go
//
// homunculi es el subsistema servidor de goblin: lo que gsh anima al
// arrancar un script, y lo que muere con él. No es un applet — no
// pasa por cmd/goblin ni por su registry, no se invoca a mano ni por
// symlink. gsh lo lanza directo, pasándole el socket NDJSON y el fd
// del pipe de muerte heredado.
package main

import (
	"fmt"
	"os"
	"strconv"

	"goblin/pkg/gutils"
)

// Uso (invocado por gsh, nunca a mano):
//
//	homunculi SOCKET_PATH DEATH_PIPE_FD
func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "homunculi: uso: homunculi SOCKET_PATH DEATH_PIPE_FD")
		os.Exit(2)
	}

	socketPath := os.Args[1]
	deathPipeFD, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "homunculi: DEATH_PIPE_FD inválido: %v\n", err)
		os.Exit(2)
	}

	if err := gutils.Run(socketPath, deathPipeFD); err != nil {
		fmt.Fprintf(os.Stderr, "homunculi: %v\n", err)
		os.Exit(1)
	}
}

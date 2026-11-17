// cmd/goblin/main.go
//
// goblin es el daemon efímero del contrato: nace con el script instructor,
// gestiona el ciclo de vida de sus workers y muere con él (o al aplicar
// on_orphan), sin persistir. No es un applet: no pasa por cmd/grimoire.
//
// Uso (lo invoca el instructor, no a mano):
//
//	goblin ID SOCKET CGROUP DEATH_FD OWNER_PID [GG_PATH]
//
// DEATH_FD es el extremo de lectura de un pipe: cuando el instructor muere,
// el kernel cierra el extremo de escritura y el goblin ve EOF, dispara la
// cascada gg kill y se va. GG_PATH permite apuntar a scripts/gg en tests.
package main

import (
	"fmt"
	"os"
	"strconv"

	"goblin/pkg/gutils"
)

func main() {
	if len(os.Args) < 6 {
		fmt.Fprintln(os.Stderr, "uso: goblin ID SOCKET CGROUP DEATH_FD OWNER_PID [GG_PATH]")
		os.Exit(2)
	}

	id := os.Args[1]
	socketPath := os.Args[2]
	cgroupPath := os.Args[3]
	deathFD, err := strconv.Atoi(os.Args[4])
	if err != nil {
		fmt.Fprintf(os.Stderr, "goblin: DEATH_FD inválido: %v\n", err)
		os.Exit(2)
	}
	ownerPID, err := strconv.Atoi(os.Args[5])
	if err != nil {
		fmt.Fprintf(os.Stderr, "goblin: OWNER_PID inválido: %v\n", err)
		os.Exit(2)
	}
	ggPath := ""
	if len(os.Args) > 6 {
		ggPath = os.Args[6]
	}

	cfg := gutils.Config{
		ID:           id,
		SocketPath:   socketPath,
		CGroupPath:   cgroupPath,
		DeathPipeFD:  deathFD,
		OwnerPID:     ownerPID,
		GGPath:       ggPath,
		OrphanPolicy: os.Getenv("GOBLIN_ORPHAN"), // keep | kill | freeze
		NotifyHook:   os.Getenv("GOBLIN_NOTIFY"), // sh -c para notify_when
		OnComplete:   os.Getenv("GOBLIN_ON_COMPLETE"),
		OnFail:       os.Getenv("GOBLIN_ON_FAIL"),
	}
	if v := os.Getenv("GOBLIN_LEFTOVER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.LeftoverSeconds = n
		}
	}
	if err := gutils.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "goblin: %v\n", err)
		os.Exit(1)
	}
}

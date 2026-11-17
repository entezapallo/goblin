// cmd/goblin/main.go
//
// Dispatcher multi-call. Reemplaza a cmd/busybox, que se borró junto con
// la poda (ver HANDOFF.md). Por ahora solo dispatchea a los 37 applets
// que sobrevivieron el recorte — sin pstree ni los verbos de goblin
// todavía, esos no existen como paquete aún.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"goblin/pkg/applets/ash"
	"goblin/pkg/applets/awk"
	"goblin/pkg/applets/cat"
	"goblin/pkg/applets/cp"
	"goblin/pkg/applets/echo"
	"goblin/pkg/applets/find"
	"goblin/pkg/applets/free"
	"goblin/pkg/applets/gunzip"
	"goblin/pkg/applets/gzip"
	"goblin/pkg/applets/head"
	"goblin/pkg/applets/kill"
	"goblin/pkg/applets/killall"
	"goblin/pkg/applets/ls"
	"goblin/pkg/applets/mkdir"
	"goblin/pkg/applets/mv"
	"goblin/pkg/applets/nice"
	"goblin/pkg/applets/nohup"
	"goblin/pkg/applets/nproc"
	"goblin/pkg/applets/pgrep"
	"goblin/pkg/applets/pidof"
	"goblin/pkg/applets/pkill"
	"goblin/pkg/applets/ps"
	"goblin/pkg/applets/pwd"
	"goblin/pkg/applets/renice"
	"goblin/pkg/applets/rm"
	"goblin/pkg/applets/rmdir"
	"goblin/pkg/applets/setsid"
	"goblin/pkg/applets/sleep"
	"goblin/pkg/applets/startstopdaemon"
	"goblin/pkg/applets/tail"
	"goblin/pkg/applets/tar"
	"goblin/pkg/applets/taskset"
	apptime "goblin/pkg/applets/time" // alias: choca con el paquete stdlib "time"
	"goblin/pkg/applets/timeout"
	"goblin/pkg/applets/uptime"
	"goblin/pkg/applets/wc"
	"goblin/pkg/applets/xargs"
	"goblin/pkg/core"
	"goblin/pkg/applets/pstree"
	"goblin/pkg/applets/goblinsubmit"
	"goblin/pkg/applets/goblinpause"
	"goblin/pkg/applets/goblinresume"
	"goblin/pkg/applets/goblinkill"
	"goblin/pkg/applets/goblinlist"
)

type runFunc func(stdio *core.Stdio, args []string) int

// registry: los 37 applets que quedaron después de la poda.
// Fuera a propósito (eliminados del árbol, ver HANDOFF.md):
// dig, nc, ss, wget, logname, users, w, who, whoami, watch, ionice, top.
// Todavía no existen como paquete (paso 2/3 del roadmap):
// pstree, goblinsubmit, goblinpause, goblinresume, goblinkill, goblinlist.
var registry = map[string]runFunc{
	"ash":             ash.Run,
	"awk":             awk.Run,
	"cat":             cat.Run,
	"cp":              cp.Run,
	"echo":            echo.Run,
	"find":            find.Run,
	"free":            free.Run,
	"gunzip":          gunzip.Run,
	"gzip":            gzip.Run,
	"head":            head.Run,
	"kill":            kill.Run,
	"killall":         killall.Run,
	"ls":              ls.Run,
	"mkdir":           mkdir.Run,
	"mv":              mv.Run,
	"nice":            nice.Run,
	"nohup":           nohup.Run,
	"nproc":           nproc.Run,
	"pgrep":           pgrep.Run,
	"pidof":           pidof.Run,
	"pkill":           pkill.Run,
	"ps":              ps.Run,
	"pwd":             pwd.Run,
	"renice":          renice.Run,
	"rm":              rm.Run,
	"rmdir":           rmdir.Run,
	"setsid":          setsid.Run,
	"sleep":           sleep.Run,
	"startstopdaemon": startstopdaemon.Run,
	"tail":            tail.Run,
	"tar":             tar.Run,
	"taskset":         taskset.Run,
	"time":            apptime.Run,
	"timeout":         timeout.Run,
	"uptime":          uptime.Run,
	"wc":              wc.Run,
	"xargs":           xargs.Run,
	"pstree":       pstree.Run,
	"goblinsubmit": goblinsubmit.Run,
	"goblinpause":  goblinpause.Run,
	"goblinresume": goblinresume.Run,
	"goblinkill":   goblinkill.Run,
	"goblinlist":   goblinlist.Run,
}

func main() {
	stdio := core.DefaultStdio()

	// Caso 1: invocado por symlink (argv[0] == nombre del applet)
	name := filepath.Base(os.Args[0])
	if run, ok := registry[name]; ok {
		os.Exit(run(stdio, os.Args[1:]))
	}

	// Caso 2: invocado como "goblin <applet> [args...]"
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: goblin <applet> [args...]")
		os.Exit(2)
	}
	name = os.Args[1]
	run, ok := registry[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "goblin: applet desconocido: %s\n", name)
		os.Exit(2)
	}
	os.Exit(run(stdio, os.Args[2:]))
}

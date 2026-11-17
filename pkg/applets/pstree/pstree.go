// pkg/applets/pstree/pstree.go
//
// Package pstree expone el árbol de procesos de gprocutil como applet
// standalone — útil para debug manual, además de ser el mismo mecanismo
// que usará pkg/goblin internamente para la recuperación de subárbol.
package pstree

import (
	"goblin/pkg/applets/gprocutil"
	"goblin/pkg/core"
	"os"
	"strconv"
)

// Run ejecuta pstree con los argumentos dados.
//
// Uso:
//
//	pstree [PID]
//
// Sin argumentos, usa el PID del proceso actual como raíz.
func Run(stdio *core.Stdio, args []string) int {
	root := os.Getpid()
	if len(args) > 0 {
		pid, err := strconv.Atoi(args[0])
		if err != nil {
			return core.UsageError(stdio, "pstree", "PID inválido: "+args[0])
		}
		root = pid
	}

	printTree(stdio, root, 0)
	return core.ExitSuccess
}

func printTree(stdio *core.Stdio, pid int, depth int) {
	for i := 0; i < depth; i++ {
		stdio.Print("  ")
	}
	stdio.Println(pid)

	for _, child := range gprocutil.Children(pid) {
		printTree(stdio, child, depth+1)
	}
}

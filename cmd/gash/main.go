// cmd/gash/main.go
//
// gash es el frontend de la DSL sobre ash: lee un .gsh, parsea el header y
// los bloques define…let…tel / grun…nurg, y corre los cuerpos con el ash
// real del fork. Cada grun se envía como un paso al goblin vía summon.
//
// Uso:
//
//	gash script.gsh
//	gash --ash FILE     (modo interno: corre FILE con ash)
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"goblin/pkg/applets/ash"
	"goblin/pkg/core"
	"goblin/pkg/gash"
)

func resolveGG() string {
	if v := os.Getenv("GOBLIN_GG"); v != "" {
		return v
	}
	exe, _ := os.Executable()
	for _, c := range []string{
		filepath.Join(filepath.Dir(exe), "scripts", "gg"),
		filepath.Join(filepath.Dir(exe), "..", "scripts", "gg"),
		filepath.Join("scripts", "gg"),
	} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return "gg"
}

func resolveSibling(name, env string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	exe, _ := os.Executable()
	c := filepath.Join(filepath.Dir(exe), name)
	if st, err := os.Stat(c); err == nil && !st.IsDir() {
		return c
	}
	return name
}

func main() {
	stdio := core.DefaultStdio()

	// Modo interno: lo invoca el goblin como worker de un paso.
	if len(os.Args) >= 3 && os.Args[1] == "--ash" {
		data, err := os.ReadFile(os.Args[2]) // #nosec G304 -- archivo temporal generado por gash
		if err != nil {
			fmt.Fprintf(os.Stderr, "gash: %v\n", err)
			os.Exit(1)
		}
		os.Exit(ash.Run(stdio, []string{"-c", string(data)}))
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: gash script.gsh")
		os.Exit(2)
	}

	script, err := gash.ParseFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "gash: %v\n", err)
		os.Exit(1)
	}

	self, _ := os.Executable()
	runner := &gash.Runner{
		GGPath:     resolveGG(),
		GoblinBin:  resolveSibling("goblin", "GOBLIN_BIN"),
		GuardBin:   resolveSibling("gguard", "GOBLIN_GUARD"),
		RuntimeDir: os.Getenv("XDG_RUNTIME_DIR"),
		AshBin:     self,
		Stdio:      stdio,
	}
	os.Exit(runner.Run(script))
}

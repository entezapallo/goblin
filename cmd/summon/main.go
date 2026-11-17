// cmd/summon/main.go
//
// summon es el launcher del contract: crea el cgroup, el socket y el pipe
// de muerte, arranca el goblin efímero, le entrega el job y espera a que
// el paso cierre (salvo con --detach).
//
// Uso:
//
//	summon [--detach] [--name ID] [--memory M] [--cpu C] [--workers N]
//	       [--orphan keep|kill|freeze] -- CMD [ARGS...]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"goblin/pkg/summon"
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

func resolveGoblin() string {
	return resolveSibling("goblin", "GOBLIN_BIN")
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
	fs := flag.NewFlagSet("summon", flag.ContinueOnError)
	detach := fs.Bool("detach", false, "devolver apenas hace submit, sin esperar el paso")
	name := fs.String("name", "", "id del goblin (por defecto, generado)")
	memory := fs.Int("memory", 0, "techo de memoria en MB")
	cpu := fs.Float64("cpu", 0, "cuota de CPU en cores")
	workers := fs.Int("workers", 0, "cantidad de workers")
	orphan := fs.String("orphan", "", "política al morir el instructor: keep|kill|freeze")
	unborn := fs.String("unborn", "", "política si el goblin no nace: keep|kill|freeze")
	headless := fs.String("headless", "", "política si el goblin muere con workers: keep|kill|freeze")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	cmd := fs.Args()
	if len(cmd) > 0 && cmd[0] == "--" {
		cmd = cmd[1:]
	}

	id, err := summon.Launch(summon.Config{
		ID:             *name,
		Cmd:            cmd,
		Detach:         *detach,
		GGPath:         resolveGG(),
		GoblinBin:      resolveGoblin(),
		GuardBin:       resolveSibling("gguard", "GOBLIN_GUARD"),
		MemoryMB:       *memory,
		CPU:            *cpu,
		Workers:        *workers,
		OrphanPolicy:   *orphan,
		UnbornPolicy:   *unborn,
		HeadlessPolicy: *headless,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "summon: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(id)
}

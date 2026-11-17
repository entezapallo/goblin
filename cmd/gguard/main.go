// cmd/gguard/main.go
//
// gguard es el supervisor de liveness de un worker. Recibe un fd de un pipe
// abierto por el goblin y el comando real. Si el pipe da EOF (el goblin murió
// y el kernel cerró sus fds), aplica on_headless: kill (SIGKILL, default),
// freeze (SIGSTOP) o keep (sin señal). Lee GOBLIN_HEADLESS del entorno.
// No conoce cgroups ni contratos: es solo mecanismo.
//
// Uso (lo invoca gutils, no a mano):
//
//	gguard LIVENESS_FD -- CMD [ARGS...]
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// headlessSignal traduce la política on_headless a una señal para el grupo.
// keep devuelve 0: no se envía nada y el worker sobrevive al goblin.
func headlessSignal(policy string) syscall.Signal {
	switch policy {
	case "freeze":
		return syscall.SIGSTOP
	case "keep":
		return 0
	default: // kill
		return syscall.SIGKILL
	}
}

func main() {
	if len(os.Args) < 4 || os.Args[2] != "--" {
		fmt.Fprintln(os.Stderr, "uso: gguard LIVENESS_FD -- CMD [ARGS...]")
		os.Exit(2)
	}
	fd, err := strconv.Atoi(os.Args[1])
	if err != nil || fd < 3 {
		fmt.Fprintf(os.Stderr, "gguard: fd inválido: %s\n", os.Args[1])
		os.Exit(2)
	}
	liveness := os.NewFile(uintptr(fd), "liveness")
	if liveness == nil {
		fmt.Fprintln(os.Stderr, "gguard: no se pudo abrir el fd de liveness")
		os.Exit(2)
	}

	cmd := exec.Command(os.Args[3], os.Args[4:]...) // #nosec G204 -- comando pedido por el goblin
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "gguard: %v\n", err)
		os.Exit(1)
	}

	result := make(chan int, 1)
	go func() {
		err := cmd.Wait()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		result <- code
	}()

	go func() {
		buf := make([]byte, 1)
		for {
			n, err := liveness.Read(buf)
			if err != nil || n == 0 {
				// El goblin murió: aplicar on_headless al grupo del hijo,
				// no solo al líder.
				if sig := headlessSignal(os.Getenv("GOBLIN_HEADLESS")); sig != 0 {
					_ = syscall.Kill(-cmd.Process.Pid, sig)
				}
				return
			}
		}
	}()

	os.Exit(<-result)
}

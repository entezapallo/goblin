// Package gutils es el core de política de goblin: qué significa cada
// evento/señal en términos de contrato de ciclo de vida. No es applet
// (no tiene Run(stdio,args), no se registra en cmd/grimoire). Es la
// librería que los applets de verbo consumen como cliente vía Call/SimpleVerb,
// y que el goblin mismo usa del lado servidor vía Serve.
package gutils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"

	"goblin/pkg/contract"
)

// State representa el estado del ciclo de vida de un goblin.
type State string

const (
	StateQueued  State = "queued"
	StateRunning State = "running"
	StatePaused  State = "paused"
	StateDone    State = "done"
	StateFailed  State = "failed"
	StateKilled  State = "killed"
)

// Terms son los términos de recurso/contrato bajo los que corre un goblin.
type Terms struct {
	MemoryMB   int     `json:"memory_mb,omitempty"`
	CPUCores   float64 `json:"cpu_cores,omitempty"`
	Workers    int     `json:"workers,omitempty"`
	MaxWPerCPU int     `json:"max_w_per_cpu,omitempty"`
}

// Event es un mensaje del protocolo NDJSON. Consultas (list/stat) NO usan
// este canal: leen cgroupfs directo. Solo las mutaciones pasan por acá.
type Event struct {
	Type     string            `json:"type"` // "submit", "pause", "resume", "kill", "list", "death", "error", "ok"
	PID      int               `json:"pid,omitempty"`
	Cmd      []string          `json:"cmd,omitempty"`
	Step     string            `json:"step,omitempty"`
	Terms    Terms             `json:"terms,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	State    State             `json:"state,omitempty"`
	Error    string            `json:"error,omitempty"`
	ExitCode int               `json:"exit_code,omitempty"`
	OOM      bool              `json:"oom,omitempty"`
}

// Config describe el goblin efímero que se va a levantar.
type Config struct {
	ID              string
	SocketPath      string
	CGroupPath      string
	DeathPipeFD     int
	OwnerPID        int
	GGPath          string          // binario gg; por defecto "gg"
	OrphanPolicy    string          // keep | kill | freeze; default keep
	NotifyHook      string          // comando sh -c para notify_when
	LeftoverSeconds int             // term antes de matar lo que quedó en el paso
	Rules           []contract.Rule // reglas aplicadas a cada paso
	OnComplete      string          // hook sh -c al cerrar un paso con éxito
	OnFail          string          // hook sh -c al cerrar un paso fallido
}

// SocketPath devuelve la ruta del socket del goblin que invoca, si se
// sobreescribe a mano. Ya no es el mecanismo de direccionamiento (usa
// pkg/manifest), pero se conserva para debugging.
func SocketPath() string {
	return os.Getenv("GOBLIN_SOCKET")
}

// Call envía un Event al socket path y espera la respuesta.
func Call(socketPath string, req Event) (Event, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return Event{}, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Event{}, err
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return Event{}, err
		}
		return Event{}, fmt.Errorf("goblin: sin respuesta del socket")
	}

	var resp Event
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return Event{}, err
	}
	return resp, nil
}

// SimpleVerb es un atajo de Call para verbos sin payload extra.
func SimpleVerb(socketPath string, verb string, pid int) (Event, error) {
	return Call(socketPath, Event{Type: verb, PID: pid})
}

// Handler procesa un Event recibido por Serve y devuelve la respuesta.
type Handler func(req Event) Event

// Serve escucha en socketPath y despacha cada Event a handle.
func Serve(socketPath string, handle Handler) error {
	if err := os.MkdirAll(dirOf(socketPath), 0700); err != nil {
		return err
	}
	_ = os.Remove(socketPath) // socket viejo de una corrida anterior

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go serveConn(conn, handle)
	}
}

func serveConn(conn net.Conn, handle Handler) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}

	var req Event
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		_ = json.NewEncoder(conn).Encode(Event{Type: "error", Error: err.Error()})
		return
	}

	_ = json.NewEncoder(conn).Encode(handle(req))
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

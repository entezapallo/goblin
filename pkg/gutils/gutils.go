// Package gutils es el core de política de goblin: qué significa cada
// evento/señal en términos de contrato de ciclo de vida. No es applet
// (no tiene Run(stdio,args), no se registra en cmd/goblin). Es la
// librería que los applets de verbo (goblinsubmit, goblinpause, etc.)
// consumen como cliente vía Call/SimpleVerb, y que el goblin mismo usa
// del lado servidor vía Serve.
package gutils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
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

// Terms son los términos de recurso/contrato bajo los que corre un goblin
// (lo que en el HANDOFF se menciona junto a resources/memory/cpu/workers).
// GPU queda fuera a propósito — es tema aparte, no entra en el core todavía.
type Terms struct {
	MemoryMB   int
	CPUCores   float64
	Workers    int
	MaxWPerCPU int
}

// Event es un mensaje del protocolo NDJSON entre Call/SimpleVerb (cliente)
// y Serve (servidor). Una línea de texto = un Event completo, tanto para
// requests como para responses. Framing simple: una conexión por llamada,
// se escribe una línea, se lee una línea, se cierra. Sin ID de correlación
// todavía — se puede agregar después sin romper el protocolo.
type Event struct {
	Type  string   `json:"type"`            // "submit", "pause", "resume", "kill", "status", "list", "done", "fail"
	PID   int      `json:"pid,omitempty"`
	Cmd   []string `json:"cmd,omitempty"`   // solo para "submit": comando a lanzar
	State State    `json:"state,omitempty"`
	Error string   `json:"error,omitempty"`
}

// SocketPath resuelve el path del unix socket del goblin actual.
// Fallback simple basado en PPID, como marca el HANDOFF: pkg/discover
// lo va a reemplazar más adelante por descubrimiento real multi-goblin
// en $XDG_RUNTIME_DIR/goblin/.
// SocketPath resuelve el path del unix socket del goblin actual.
// Si GOBLIN_SOCKET está seteada, se usa tal cual (override manual, útil
// para debugging/pruebas). Si no, fallback simple basado en PPID, como
// marca el HANDOFF: pkg/discover lo va a reemplazar más adelante por
// descubrimiento real multi-goblin en $XDG_RUNTIME_DIR/goblin/.
func SocketPath() string {
	if override := os.Getenv("GOBLIN_SOCKET"); override != "" {
		return override
	}

	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return fmt.Sprintf("%s/goblin/%d.sock", runtimeDir, os.Getppid())
}// Call envía un Event al goblin en socketPath y espera la respuesta.
// Una conexión por llamada: conecta, escribe una línea JSON, lee una
// línea JSON de vuelta, cierra.
func Call(socketPath string, req Event) (Event, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return Event{}, err
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	if err := enc.Encode(req); err != nil {
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

// SimpleVerb es un atajo de Call para verbos sin payload extra
// (pause/resume/kill/list), usado por los applets de verbo.
func SimpleVerb(socketPath string, verb string, pid int) (Event, error) {
	return Call(socketPath, Event{Type: verb, PID: pid})
}

// Handler procesa un Event recibido por Serve y devuelve la respuesta.
// El goblin real lo implementa con la lógica de estados/cascada; esto
// es solo la firma del contrato entre Serve y esa lógica.
type Handler func(req Event) Event

// Serve escucha en socketPath y despacha cada Event recibido a handle,
// una conexión a la vez, hasta que la conexión se cierra o falla.
// No maneja Spawn/fork-exec — eso es responsabilidad de handle.
func Serve(socketPath string, handle Handler) error {
	if err := os.MkdirAll(dirOf(socketPath), 0700); err != nil {
		return err
	}
	os.Remove(socketPath) // por si quedó un socket viejo de una corrida anterior

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
		resp := Event{Type: "error", Error: err.Error()}
		json.NewEncoder(conn).Encode(resp)
		return
	}

	resp := handle(req)
	json.NewEncoder(conn).Encode(resp)
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

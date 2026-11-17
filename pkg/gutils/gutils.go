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
	"sync"
	"time"

	"goblin/pkg/contract"
)

const connectTimeout = 5 * time.Second

// callTimeout es var para poder acortarlo en los tests de timeout.
var callTimeout = 10 * time.Second

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
	MemoryMB int     `json:"memory_mb,omitempty"`
	CPUCores float64 `json:"cpu_cores,omitempty"`
	Workers  int     `json:"workers,omitempty"`
}

// StepInfo es el resumen tipado de un paso en una respuesta de list.
type StepInfo struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Outcome string `json:"outcome,omitempty"`
	Workers int    `json:"workers"`
}

// Event es un mensaje del protocolo NDJSON. Consultas (list/stat) NO usan
// este canal: leen cgroupfs directo. Solo las mutaciones pasan por acá.
// Error es SOLO para errores; list usa Steps, output usa Line, y los
// mensajes informativos usan Message. Antes todo se colaba en Error.
type Event struct {
	Type       string              `json:"type"` // "submit", "pause", "resume", "kill", "list", "death", "error", "ok"
	PID        int                 `json:"pid,omitempty"`
	Cmd        []string            `json:"cmd,omitempty"`
	Step       string              `json:"step,omitempty"`
	Terms      Terms               `json:"terms,omitempty"`
	Labels     map[string]string   `json:"labels,omitempty"`
	Rules      []contract.RuleSpec `json:"rules,omitempty"`
	OnComplete string              `json:"on_complete,omitempty"`
	OnFail     string              `json:"on_fail,omitempty"`
	State      State               `json:"state,omitempty"`
	Error      string              `json:"error,omitempty"`   // solo Type:"error"
	Message    string              `json:"message,omitempty"` // informativo (death, ok)
	Line       string              `json:"line,omitempty"`    // texto de stdout/stderr
	Steps      []StepInfo          `json:"steps,omitempty"`   // respuesta de list
	ExitCode   int                 `json:"exit_code,omitempty"`
	OOM        bool                `json:"oom,omitempty"`
	Stream     string              `json:"stream,omitempty"` // stdout|stderr, eventos internos de salida
}

// Config describe el goblin efímero que se va a levantar.
type Config struct {
	ID              string
	SocketPath      string
	CGroupPath      string
	DeathPipeFD     int
	OwnerPID        int
	GGPath          string          // binario gg; por defecto "gg"
	GuardBin        string          // supervisor de liveness; por defecto "gguard"
	OrphanPolicy    string          // keep | kill | freeze; default keep
	NotifyHook      string          // canal default de notify_when
	RuntimeDir      string          // XDG_RUNTIME_DIR para notificaciones/historial
	LeftoverSeconds int             // term antes de matar lo que quedó en el paso
	Rules           []contract.Rule // reglas aplicadas a cada paso
	OnComplete      string          // hook sh -c al cerrar un paso con éxito
	OnFail          string          // hook sh -c al cerrar un paso fallido
}

// Call envía un Event al socket path y espera la respuesta. Si el goblin
// está colgado (por ejemplo en un gg lento), falla en vez de bloquear para
// siempre a los verbos o a gash.
func Call(socketPath string, req Event) (Event, error) {
	conn, err := net.DialTimeout("unix", socketPath, connectTimeout)
	if err != nil {
		return Event{}, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(callTimeout)); err != nil {
		return Event{}, err
	}

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

// Serve escucha en socketPath y despacha cada Event a handle. wg cuenta
// las conexiones en vuelo para que Run no salga (y mate el proceso) antes
// de que se escriba la última respuesta.
func Serve(socketPath string, handle Handler, wg *sync.WaitGroup) error {
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
		if wg != nil {
			wg.Add(1)
		}
		go func() {
			if wg != nil {
				defer wg.Done()
			}
			serveConn(conn, handle)
		}()
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

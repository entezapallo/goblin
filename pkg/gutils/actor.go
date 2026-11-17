// El actor: única goroutine dueña del mapa de workers de este goblin.
// Todo lo demás (conexiones NDJSON vía Serve, el watcher de muerte del
// script) le manda eventos por channel — nadie más toca el mapa directo.
package gutils

import (
	"goblin/pkg/applets/gprocutil"
	"io"
	"os"
	"strconv"
	"syscall"
)

// worker es lo que el actor sabe de un job lanzado por submit.
// pid sirve también como pgid: se lanza con Setpgid:true sin Pgid
// explícito, así que el kernel hace pgid == pid del líder.
type worker struct {
	pid   int
	state State
}

// cmd es un mensaje interno: un Event de entrada más el channel donde
// el actor deja la respuesta. reply siempre tiene buffer 1, así que un
// emisor que no lee la respuesta (como watchDeathPipe) no bloquea al actor.
type cmd struct {
	event Event
	reply chan Event
}

// actor es el loop dueño del estado. Se arranca una vez por goblin
// (una vez por script gsh), vive mientras no haya procesado un "death".
type actor struct {
	cmds    chan cmd
	stopped chan struct{} // se cierra cuando el actor termina el loop
}

func newActor() *actor {
	return &actor{
		cmds:    make(chan cmd),
		stopped: make(chan struct{}),
	}
}

// Handle es el Handler que se le pasa a Serve: empaqueta el Event como
// cmd, lo mete en la cola del actor, espera la respuesta. Si el actor
// ya terminó (cascada de death ya corrida), devuelve error en vez de
// bloquear para siempre.
func (a *actor) Handle(req Event) Event {
	reply := make(chan Event, 1)
	select {
	case a.cmds <- cmd{event: req, reply: reply}:
		return <-reply
	case <-a.stopped:
		return Event{Type: "error", Error: "goblin: ya terminó"}
	}
}

// notify es lo que usa watchDeathPipe: mete un evento sin esperar
// respuesta. Mismo guard contra actor ya detenido.
func (a *actor) notify(evt Event) {
	select {
	case a.cmds <- cmd{event: evt, reply: make(chan Event, 1)}:
	case <-a.stopped:
	}
}

// run es el loop principal. Procesa un cmd a la vez, en orden de
// llegada — esto es lo que garantiza que la cascada de "death" no se
// entrelace con un submit/pause/kill a mitad de camino.
func (a *actor) run() {
	defer close(a.stopped)
	workers := make(map[int]*worker)

	for c := range a.cmds {
		var resp Event

		switch c.event.Type {
		case "submit":
			proc, err := Spawn(c.event.Cmd)
			if err != nil {
				resp = Event{Type: "error", Error: err.Error()}
				break
			}
			pid := proc.Process.Pid
			workers[pid] = &worker{pid: pid, state: StateRunning}
			resp = Event{Type: "submit", PID: pid, State: StateRunning}

		case "pause":
			resp = signalWorker(workers, c.event.PID, syscall.SIGSTOP, StatePaused)

		case "resume":
			resp = signalWorker(workers, c.event.PID, syscall.SIGCONT, StateRunning)

		case "kill":
			resp = killWorker(workers, c.event.PID)

		case "list":
			resp = Event{Type: "list", Error: listSummary(workers)}

		case "death":
			killAll(workers)
			resp = Event{Type: "death", Error: "cascada completa"}
			c.reply <- resp
			return // termina el loop → cierra a.stopped vía defer

		default:
			resp = Event{Type: "error", Error: "verbo desconocido: " + c.event.Type}
		}

		c.reply <- resp
	}
}

// signalWorker manda sig al grupo de proceso del worker (pid == pgid) y
// actualiza su estado si el signal se entregó bien.
func signalWorker(workers map[int]*worker, pid int, sig syscall.Signal, newState State) Event {
	w, ok := workers[pid]
	if !ok {
		return Event{Type: "error", Error: "pid desconocido"}
	}
	if err := syscall.Kill(-w.pid, sig); err != nil {
		return Event{Type: "error", PID: pid, Error: err.Error()}
	}
	w.state = newState
	return Event{Type: "ok", PID: pid, State: w.state}
}

// killWorker mata el grupo de proceso del worker y, por si algo se
// reparentó fuera del grupo, también su subárbol real vía gprocutil.
func killWorker(workers map[int]*worker, pid int) Event {
	w, ok := workers[pid]
	if !ok {
		return Event{Type: "error", Error: "pid desconocido"}
	}
	killTree(w.pid)
	w.state = StateKilled
	return Event{Type: "ok", PID: pid, State: w.state}
}

// killAll cascadea kill sobre todos los workers vivos — lo que dispara
// el evento "death" (muerte del script gsh, detectada por pipe-EOF).
func killAll(workers map[int]*worker) {
	for pid, w := range workers {
		if w.state == StateKilled || w.state == StateDone || w.state == StateFailed {
			continue
		}
		killTree(pid)
		w.state = StateKilled
	}
}

// killTree mata el grupo de proceso del líder y, aparte, cada
// descendiente real reportado por gprocutil — cubre el caso de un
// proceso que se salió de su grupo original (setsid, doble fork, etc.)
// y por lo tanto no muere con el -pgid del líder.
func killTree(leaderPID int) {
	syscall.Kill(-leaderPID, syscall.SIGKILL) // grupo del líder
	for _, pid := range gprocutil.Descendants(leaderPID) {
		syscall.Kill(pid, syscall.SIGKILL) // por si quedó fuera del grupo
	}
}

// listSummary es un placeholder de serialización — falta decidir si
// Event necesita un campo propio para listas (hoy se pisa Error a
// falta de algo mejor, mismo parche temporal que tuvo Cmd al principio).
func listSummary(workers map[int]*worker) string {
	if len(workers) == 0 {
		return "(sin workers)"
	}
	out := ""
	for pid, w := range workers {
		out += strconv.Itoa(pid) + ":" + string(w.state) + " "
	}
	return out
}

// watchDeathPipe se queda bloqueada leyendo deathPipe. El kernel cierra
// ese fd solo cuando TODOS los procesos que lo tenían abierto mueren —
// incluido el script gsh muerto por SIGKILL sin chance de correr nada.
// Al ver EOF, notifica al actor y termina.
func watchDeathPipe(deathPipe *os.File, a *actor) {
	buf := make([]byte, 1)
	for {
		_, err := deathPipe.Read(buf)
		if err == io.EOF {
			a.notify(Event{Type: "death"})
			return
		}
		if err != nil {
			a.notify(Event{Type: "death"})
			return
		}
		// No debería llegar data nunca — si llega, se ignora y se sigue
		// esperando EOF.
	}
}

// Run arranca las 3 goroutines del goblin: el actor, Serve escuchando
// el socket NDJSON, y watchDeathPipe vigilando el fd heredado del
// script gsh. Termina en cuanto Serve falle, o en cuanto el actor
// procese "death" y cierre stopped — en ese caso el goblin se limpia
// y sale solo, sin depender de que nadie esté mirando (ni el TUI ni
// nadie más). Eso es lo que evita que el proceso goblin mismo quede
// huérfano después de que ya mató a todos sus workers.
func Run(socketPath string, deathPipeFD int) error {
	a := newActor()
	go a.run()

	deathPipe := os.NewFile(uintptr(deathPipeFD), "deathpipe")
	go watchDeathPipe(deathPipe, a)

	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(socketPath, a.Handle) }()

	select {
	case err := <-serveErr:
		return err
	case <-a.stopped:
		os.Remove(socketPath)
		return nil
	}
}

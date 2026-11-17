// pkg/applets/gprocutil/gprocutil.go
//
// Package gprocutil promueve el parsing de /proc/PID/stat que ya usaba
// pkg/applets/ps (readStat, sin exportar) a un paquete compartido.
// La identidad de un proceso es (pid, starttime): el PID se recicla, el
// starttime (campo 22 de /proc/PID/stat) no.
package gprocutil

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadStat lee y parsea /proc/PID/stat. Devuelve starttime como último valor.
func ReadStat(pid int) (ppid, pgid, sid, ttyNr int, state string, nice, vsizeKB, rssKB int, starttime uint64) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")) // #nosec G304 -- /proc read
	if err != nil {
		return 0, 0, 0, 0, "", 0, 0, 0, 0
	}
	stat := strings.TrimSpace(string(data))
	closeIdx := strings.LastIndex(stat, ") ")
	if closeIdx == -1 {
		return 0, 0, 0, 0, "", 0, 0, 0, 0
	}
	rest := strings.Fields(stat[closeIdx+2:])
	if len(rest) < 22 {
		return 0, 0, 0, 0, "", 0, 0, 0, 0
	}
	ppid = parseInt(rest[1])
	pgid = parseInt(rest[2])
	sid = parseInt(rest[3])
	ttyNr = parseInt(rest[4])
	state = rest[0]
	nice = parseInt(rest[16])
	vsizeKB = parseInt(rest[20]) / 1024
	rssKB = parseInt(rest[21]) * os.Getpagesize() / 1024
	starttime, _ = strconv.ParseUint(rest[19], 10, 64)
	return ppid, pgid, sid, ttyNr, state, nice, vsizeKB, rssKB, starttime
}

func parseInt(value string) int {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return int(parsed)
}

// Identity identifica un proceso sin ambigüedad de reciclaje de PID.
type Identity struct {
	PID       int
	Starttime uint64
}

// IdentityOf devuelve la identidad de un proceso vivo. ok=false si ya no
// existe o no se pudo leer su /proc/PID/stat.
func IdentityOf(pid int) (Identity, bool) {
	_, _, _, _, _, _, _, _, starttime := ReadStat(pid)
	if starttime == 0 {
		return Identity{}, false
	}
	return Identity{PID: pid, Starttime: starttime}, true
}

// Ancestors sube por el PPID real desde pid hasta init (PID 1), capturando
// la identidad de cada eslabón. Guarda contra ciclos y procesos que
// desaparecen durante el recorrido. No incluye el propio pid.
func Ancestors(pid int) []Identity {
	var out []Identity
	seen := make(map[int]bool)
	ppid, _, _, _, _, _, _, _, _ := ReadStat(pid)
	// init (PID 1) no se incluye: su starttime marca el arranque del
	// contenedor, no una identidad útil para el contract.
	for ppid > 1 {
		if seen[ppid] {
			break
		}
		seen[ppid] = true
		id, ok := IdentityOf(ppid)
		if !ok {
			break
		}
		out = append(out, id)
		next, _, _, _, _, _, _, _, _ := ReadStat(ppid)
		if next <= 0 || next >= ppid {
			break
		}
		ppid = next
	}
	return out
}

// Subtree devuelve el subárbol BFS desde root, incluyéndolo, con la
// identidad (pid,starttime) capturada en un solo pase por /proc.
func Subtree(root int) []Identity {
	rootID, ok := IdentityOf(root)
	if !ok {
		return nil
	}
	childrenOf := make(map[int][]Identity)
	for _, candidate := range allPIDs() {
		ppid, _, _, _, _, _, _, _, starttime := ReadStat(candidate)
		if ppid <= 0 || starttime == 0 {
			continue
		}
		childrenOf[ppid] = append(childrenOf[ppid], Identity{PID: candidate, Starttime: starttime})
	}

	var result []Identity
	queue := []Identity{rootID}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		result = append(result, next)
		queue = append(queue, childrenOf[next.PID]...)
	}
	return result
}

// allPIDs enumera todos los PIDs numéricos actualmente en /proc.
func allPIDs() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	pids := make([]int, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			if pid, err := strconv.Atoi(e.Name()); err == nil {
				pids = append(pids, pid)
			}
		}
	}
	return pids
}

// Children devuelve los hijos directos de pid (PPID == pid).
func Children(pid int) []int {
	var children []int
	for _, candidate := range allPIDs() {
		ppid, _, _, _, _, _, _, _, _ := ReadStat(candidate)
		if ppid == pid {
			children = append(children, candidate)
		}
	}
	return children
}

// Descendants devuelve todo el subárbol de pid en orden BFS, con un solo
// pase por /proc para construir el mapa de PPIDs.
func Descendants(pid int) []int {
	childrenOf := make(map[int][]int)
	for _, candidate := range allPIDs() {
		ppid, _, _, _, _, _, _, _, _ := ReadStat(candidate)
		childrenOf[ppid] = append(childrenOf[ppid], candidate)
	}

	var result []int
	queue := childrenOf[pid]
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		result = append(result, next)
		queue = append(queue, childrenOf[next]...)
	}
	return result
}

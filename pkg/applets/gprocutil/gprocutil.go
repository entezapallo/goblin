// pkg/applets/gprocutil/gprocutil.go
//
// Package gprocutil promueve el parsing de /proc/PID/stat que ya usaba
// pkg/applets/ps (readStat, sin exportar) a un paquete compartido, para
// que pkg/goblin y pstree lo puedan usar sin reimplementarlo. Vive junto
// a pkg/applets/procutil (mismo patrón: helper compartido colocado con
// los applets, no en pkg/core).
package gprocutil

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadStat lee y parsea /proc/PID/stat.
// Copia literal de ps.readStat (pkg/applets/ps/ps.go:227), promovida
// acá para poder compartirse entre paquetes.
func ReadStat(pid int) (ppid, pgid, sid, ttyNr int, state string, nice, vsizeKB, rssKB int) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")) // #nosec G304 -- /proc read
	if err != nil {
		return 0, 0, 0, 0, "", 0, 0, 0
	}
	stat := strings.TrimSpace(string(data))
	closeIdx := strings.LastIndex(stat, ") ")
	if closeIdx == -1 {
		return 0, 0, 0, 0, "", 0, 0, 0
	}
	rest := strings.Fields(stat[closeIdx+2:])
	if len(rest) < 22 {
		return 0, 0, 0, 0, "", 0, 0, 0
	}
	ppid = parseInt(rest[1])
	pgid = parseInt(rest[2])
	sid = parseInt(rest[3])
	ttyNr = parseInt(rest[4])
	state = rest[0]
	nice = parseInt(rest[16])
	vsizeKB = parseInt(rest[20]) / 1024
	rssKB = parseInt(rest[21]) * os.Getpagesize() / 1024
	return ppid, pgid, sid, ttyNr, state, nice, vsizeKB, rssKB
}

func parseInt(value string) int {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return int(parsed)
}

// allPIDs enumera todos los PIDs numéricos actualmente en /proc.
func allPIDs() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	pids := make([]int, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if pid, err := strconv.Atoi(e.Name()); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

// Children devuelve los hijos directos de pid (PPID == pid).
func Children(pid int) []int {
	var children []int
	for _, candidate := range allPIDs() {
		ppid, _, _, _, _, _, _, _ := ReadStat(candidate)
		if ppid == pid {
			children = append(children, candidate)
		}
	}
	return children
}

// Descendants devuelve todo el subárbol de pid (hijos, nietos, etc.),
// en orden BFS. Un solo pase por /proc para construir el mapa completo,
// en vez de llamar Children() repetidas veces.
func Descendants(pid int) []int {
	childrenOf := make(map[int][]int)
	for _, candidate := range allPIDs() {
		ppid, _, _, _, _, _, _, _ := ReadStat(candidate)
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

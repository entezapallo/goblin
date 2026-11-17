package gprocutil_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"goblin/pkg/applets/gprocutil"
)

func TestIdentityOfSelf(t *testing.T) {
	id, ok := gprocutil.IdentityOf(os.Getpid())
	if !ok || id.PID != os.Getpid() || id.Starttime == 0 {
		t.Fatalf("identity=%+v ok=%v", id, ok)
	}
	if _, ok := gprocutil.IdentityOf(-1); ok {
		t.Fatal("PID inexistente devolvió identidad")
	}
}

func TestAncestors(t *testing.T) {
	ancestors := gprocutil.Ancestors(os.Getpid())
	if len(ancestors) == 0 {
		t.Fatal("sin ancestros")
	}
	if ancestors[0].PID != os.Getppid() {
		t.Fatalf("primer ancestro=%d, quería ppid=%d", ancestors[0].PID, os.Getppid())
	}
	seen := make(map[int]bool)
	for _, a := range ancestors {
		if a.Starttime == 0 {
			t.Fatalf("ancestro sin starttime: %+v", a)
		}
		if seen[a.PID] {
			t.Fatalf("ciclo en PID %d", a.PID)
		}
		seen[a.PID] = true
		if a.PID <= 1 {
			t.Fatalf("init incluido en la cadena: %+v", a)
		}
	}
}

func TestSubtree(t *testing.T) {
	tree := gprocutil.Subtree(os.Getpid())
	if len(tree) == 0 {
		t.Fatal("subtree vacío")
	}
	if tree[0].PID != os.Getpid() || tree[0].Starttime == 0 {
		t.Fatalf("raíz del subtree=%+v", tree[0])
	}
	for _, n := range tree[1:] {
		ppid, _, _, _, _, _, _, _, _ := gprocutil.ReadStat(n.PID)
		if ppid <= 0 {
			t.Fatalf("nodo sin ppid: %+v", n)
		}
	}
	if got := gprocutil.Subtree(-1); len(got) != 0 {
		t.Fatalf("subtree de PID inexistente=%+v", got)
	}
}

func TestReadStatStarttime(t *testing.T) {
	pid := os.Getpid()
	_, _, _, _, _, _, _, _, starttime := gprocutil.ReadStat(pid)
	if starttime == 0 {
		t.Fatal("starttime es cero")
	}

	// Verificación independiente: campo 22 de /proc/self/stat.
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		t.Skip(err)
	}
	stat := strings.TrimSpace(string(data))
	rest := strings.Fields(stat[strings.LastIndex(stat, ") ")+2:])
	want, _ := strconv.ParseUint(rest[19], 10, 64)
	if starttime != want {
		t.Fatalf("starttime=%d, want=%d", starttime, want)
	}
}

func TestIdentityIsStable(t *testing.T) {
	pid := os.Getpid()
	_, _, _, _, _, _, _, _, first := gprocutil.ReadStat(pid)
	_, _, _, _, _, _, _, _, second := gprocutil.ReadStat(pid)
	if first != second || first == 0 {
		t.Fatalf("identidad inestable: %d vs %d", first, second)
	}
}

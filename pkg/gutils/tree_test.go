package gutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFakeProc(t *testing.T, root string, pid int, ppid int, starttime uint64, cmdline string) {
	t.Helper()
	dir := filepath.Join(root, "proc", fmt.Sprint(pid))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// El stat necesita 22 campos después de comm: state(3), ppid(4), ... starttime(22).
	fields := make([]string, 22)
	fields[0] = "S" // state
	fields[1] = fmt.Sprint(ppid)
	fields[19] = fmt.Sprint(starttime) // campo 22 del stat
	for i := range fields {
		if fields[i] == "" {
			fields[i] = "0"
		}
	}
	stat := fmt.Sprintf("%d (fake) %s", pid, strings.Join(fields, " "))
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline+"\x00arg"), 0600); err != nil {
		t.Fatal(err)
	}
}

func setupTree(t *testing.T) (procRoot, stepCgroup string) {
	t.Helper()
	base := t.TempDir()
	procRoot = filepath.Join(base, "proc")
	stepCgroup = filepath.Join(base, "step-1")
	if err := os.MkdirAll(filepath.Join(stepCgroup, "w-1"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stepCgroup, "w-1", "cgroup.procs"), []byte("100\n101\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stepCgroup, "cgroup.events"), []byte("populated 1\nfrozen 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	writeFakeProc(t, base, 100, 1, 111, "bwa")
	writeFakeProc(t, base, 101, 100, 222, "samtools view")
	return procRoot, stepCgroup
}

func TestTreeSamplerSample(t *testing.T) {
	procRoot, stepCgroup := setupTree(t)
	s := NewTreeSampler(procRoot, stepCgroup, "step-1")
	s.Sample()
	snap := s.Snapshot()
	if len(snap.Processes) != 2 {
		t.Fatalf("processes=%d (%+v)", len(snap.Processes), snap.Processes)
	}
	byPID := make(map[int]ProcNode)
	for _, p := range snap.Processes {
		byPID[p.PID] = p
	}
	w := byPID[100]
	if w.Starttime != 111 || w.Argv0 != "bwa" || w.Worker != "w-1" || w.PPID != 1 {
		t.Fatalf("node=%+v", w)
	}
	if byPID[101].Starttime != 222 || byPID[101].PPID != 100 {
		t.Fatalf("child=%+v", byPID[101])
	}
	if snap.Step != "step-1" || snap.SampledAt.IsZero() || snap.Truncated != 0 {
		t.Fatalf("snap=%+v", snap)
	}
}

func TestTreeSamplerIdentityOnRecycle(t *testing.T) {
	procRoot, stepCgroup := setupTree(t)
	s := NewTreeSampler(procRoot, stepCgroup, "step-1")
	s.Sample()
	// El PID 100 cambia de starttime: debe contarse como otro proceso.
	writeFakeProc(t, filepath.Dir(procRoot), 100, 1, 999, "newcmd")
	s.Sample()
	snap := s.Snapshot()
	if len(snap.Processes) != 3 {
		t.Fatalf("esperaba 3 identidades, got %d", len(snap.Processes))
	}
}

func TestTreeSamplerShouldSample(t *testing.T) {
	procRoot, stepCgroup := setupTree(t)
	s := NewTreeSampler(procRoot, stepCgroup, "step-1")
	now := time.Now()
	if !s.ShouldSample(now) {
		t.Fatal("el primer muestreo siempre debe disparar")
	}
	if s.ShouldSample(now.Add(time.Second)) {
		t.Fatal("sin cambio no debe disparar")
	}
	if !s.ShouldSample(now.Add(6 * time.Minute)) {
		t.Fatal("el resguardo de 5 min debe disparar")
	}
	// Cambio de populated dispara.
	if err := os.WriteFile(filepath.Join(stepCgroup, "cgroup.events"), []byte("populated 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !s.ShouldSample(now.Add(6*time.Minute + time.Second)) {
		t.Fatal("el cambio de populated debe disparar")
	}
}

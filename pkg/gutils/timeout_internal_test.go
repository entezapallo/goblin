package gutils

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Un goblin colgado no debe bloquear a los verbos para siempre.
func TestCallTimeout(t *testing.T) {
	old := callTimeout
	callTimeout = 200 * time.Millisecond
	defer func() { callTimeout = old }()

	socket := filepath.Join(t.TempDir(), "hang.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			time.Sleep(2 * time.Second) // nunca responde
			_ = conn.Close()
		}
	}()

	start := time.Now()
	_, err = Call(socket, Event{Type: "list"})
	if err == nil {
		t.Fatal("se esperaba timeout")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Call tardó %s en vez de ~200ms", elapsed)
	}
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("error inesperado: %v", err)
	}
}

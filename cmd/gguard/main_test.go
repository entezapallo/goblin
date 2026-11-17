package main

import (
	"syscall"
	"testing"
)

func TestHeadlessSignal(t *testing.T) {
	cases := []struct {
		policy string
		want   syscall.Signal
	}{
		{"kill", syscall.SIGKILL},
		{"", syscall.SIGKILL}, // default
		{"otro", syscall.SIGKILL},
		{"freeze", syscall.SIGSTOP},
		{"keep", 0},
	}
	for _, c := range cases {
		if got := headlessSignal(c.policy); got != c.want {
			t.Fatalf("headlessSignal(%q)=%v, want %v", c.policy, got, c.want)
		}
	}
}

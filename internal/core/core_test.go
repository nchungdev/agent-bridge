package core

import "testing"

func TestTransitions(t *testing.T) {
	ok := [][2]State{{StateCreated, StateStarting}, {StateIdle, StateRunning}, {StateRunning, StateAwaitingApproval}, {StateAwaitingApproval, StateSuspended}, {StateSuspended, StateResuming}, {StateFailed, StateStarting}}
	bad := [][2]State{{StateCreated, StateRunning}, {StateIdle, StateAwaitingApproval}, {StateStopped, StateIdle}, {StateSuspended, StateRunning}}
	for _, p := range ok {
		if !CanTransition(p[0], p[1]) {
			t.Errorf("%s->%s should be allowed", p[0], p[1])
		}
	}
	for _, p := range bad {
		if CanTransition(p[0], p[1]) {
			t.Errorf("%s->%s should be refused", p[0], p[1])
		}
	}
}

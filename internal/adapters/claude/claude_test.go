package claude_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nchungdev/agent-hub/internal/adapters/claude"
	"github.com/nchungdev/agent-hub/internal/core"
)

func collect(t *testing.T, s core.Session, until core.EventType, onApproval func(core.Event)) []core.Event {
	t.Helper()
	var out []core.Event
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				t.Fatalf("closed early: %+v", out)
			}
			out = append(out, ev)
			if ev.Type == core.EvApprovalRequest && onApproval != nil {
				onApproval(ev)
			}
			if ev.Type == until {
				return out
			}
		case <-timeout:
			t.Fatalf("timeout: %+v", out)
		}
	}
}

func start(t *testing.T, resume string) core.Session {
	e := claude.New("testdata/fakeclaude.py")
	s, err := e.Start(context.Background(), core.StartOpts{ResumeID: resume, Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestTextAndUsage(t *testing.T) {
	s := start(t, "")
	if s.EngineSessionID() == "" {
		t.Fatal("no session id")
	}
	_ = s.Send(context.Background(), core.UserInput{Text: "hi"})
	evs := collect(t, s, core.EvTurnDone, nil)
	text := ""
	for _, e := range evs {
		if e.Type == core.EvTextDelta {
			text += e.Text
		}
	}
	if text != "hello " { // partial delta only; final assistant text must not duplicate
		t.Fatalf("text=%q", text)
	}
	last := evs[len(evs)-1]
	if last.Usage == nil || last.Usage.OutputTokens != 40 || last.Usage.ContextTokens != 20+600+100+35 || last.Usage.ContextWindow != 200000 {
		t.Fatalf("usage=%+v", last.Usage)
	}
}

func TestApprovalRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		allow bool
		want  string
	}{{true, "ran"}, {false, "denied: nope"}} {
		s := start(t, "")
		_ = s.Send(context.Background(), core.UserInput{Text: "TOOL"})
		evs := collect(t, s, core.EvTurnDone, func(e core.Event) {
			if e.Approval.Tool != "Bash" || e.Approval.Risk != "high" || e.Approval.Args["command"] != "ls" {
				t.Errorf("approval = %+v", e.Approval)
			}
			if err := s.Decide(e.Approval.ID, core.Decision{Allow: tc.allow, Reason: "nope"}); err != nil {
				t.Error(err)
			}
		})
		var res string
		for _, e := range evs {
			if e.Type == core.EvToolResult {
				res = e.Tool.Output
			}
		}
		if res != tc.want {
			t.Fatalf("allow=%v result=%q want %q", tc.allow, res, tc.want)
		}
	}
}

func TestAuthErrorClassified(t *testing.T) {
	s := start(t, "")
	_ = s.Send(context.Background(), core.UserInput{Text: "NOTLOGGED"})
	evs := collect(t, s, core.EvError, nil)
	if evs[len(evs)-1].Err.Kind != "auth" {
		t.Fatalf("%+v", evs[len(evs)-1].Err)
	}
}

func TestResumeKeepsID(t *testing.T) {
	s := start(t, "abc-123")
	if s.EngineSessionID() != "abc-123" {
		t.Fatalf("id=%s", s.EngineSessionID())
	}
}

func TestPreambleAndSetModeAndCancel(t *testing.T) {
	s := start(t, "")
	if err := s.SetMode("plan"); err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	_ = s.Send(context.Background(), core.UserInput{Text: "hi", Preamble: "earlier stuff"})
	collect(t, s, core.EvTurnDone, nil)
}

func TestCommandsViaInitialize(t *testing.T) {
	abs, _ := filepath.Abs("testdata/fakeclaude.py")
	cmds, err := claude.New(abs).Commands(context.Background())
	if err != nil || len(cmds) != 2 || cmds[0].Name != "compact" || cmds[1].ArgHint != "<file>" {
		t.Fatalf("cmds=%+v err=%v", cmds, err)
	}
}

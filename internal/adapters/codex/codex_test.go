package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nchungdev/agent-hub/internal/adapters/codex"
	"github.com/nchungdev/agent-hub/internal/core"
)

func start(t *testing.T, resume string) core.Session {
	s, err := codex.New("testdata/fakecodex.py").Start(context.Background(), core.StartOpts{ResumeID: resume, Mode: "ask", Workspace: "."})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

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

func TestTextAndUsage(t *testing.T) {
	s := start(t, "")
	if s.EngineSessionID() != "thr-1" {
		t.Fatalf("id=%s", s.EngineSessionID())
	}
	if err := s.Send(context.Background(), core.UserInput{Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	evs := collect(t, s, core.EvTurnDone, nil)
	last := evs[len(evs)-1]
	if last.Usage == nil || last.Usage.OutputTokens != 6 || last.Usage.ContextTokens != 11 || last.Usage.ContextWindow != 258000 {
		t.Fatalf("usage=%+v", last.Usage)
	}
}

func TestCommandApprovalAllowDeny(t *testing.T) {
	for _, tc := range []struct {
		d    core.Decision
		want string
	}{{core.Decision{Allow: true}, "decision=accept"}, {core.Decision{Allow: true, Scope: "session"}, "decision=acceptForSession"}, {core.Decision{Allow: false}, "decision=decline"}} {
		s := start(t, "")
		_ = s.Send(context.Background(), core.UserInput{Text: "CMD"})
		evs := collect(t, s, core.EvTurnDone, func(e core.Event) {
			if e.Approval.Tool != "Bash" || e.Approval.Args["command"] != "ls /" {
				t.Errorf("approval=%+v", e.Approval)
			}
			if err := s.Decide(e.Approval.ID, tc.d); err != nil {
				t.Error(err)
			}
		})
		got := ""
		for _, e := range evs {
			if e.Type == core.EvToolResult {
				got = e.Tool.Output
			}
		}
		if got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}

func TestFileChangeApprovalAndDiff(t *testing.T) {
	s := start(t, "")
	_ = s.Send(context.Background(), core.UserInput{Text: "FILE"})
	evs := collect(t, s, core.EvTurnDone, func(e core.Event) {
		if e.Approval.Tool != "Edit" {
			t.Errorf("tool=%s", e.Approval.Tool)
		}
		_ = s.Decide(e.Approval.ID, core.Decision{Allow: true})
	})
	found := false
	for _, e := range evs {
		if e.Type == core.EvDiff && e.Diff.File == "a.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("no diff event")
	}
}

func TestFailedTurnClassified(t *testing.T) {
	s := start(t, "")
	_ = s.Send(context.Background(), core.UserInput{Text: "FAIL"})
	evs := collect(t, s, core.EvError, nil)
	if evs[len(evs)-1].Err.Kind != "auth" {
		t.Fatalf("%+v", evs[len(evs)-1].Err)
	}
}

func TestResumeUsesThreadID(t *testing.T) {
	s := start(t, "thr-xyz")
	if s.EngineSessionID() != "thr-xyz" {
		t.Fatalf("id=%s", s.EngineSessionID())
	}
}

func TestModelsAndStatus(t *testing.T) {
	e := codex.New("testdata/fakecodex.py")
	ms, _ := e.Models(context.Background())
	if len(ms) != 2 || ms[0].ID != "fake-1" {
		t.Fatalf("models=%+v", ms)
	}
	st := e.Status(context.Background())
	if !st.Known || st.LoggedIn {
		t.Fatalf("status=%+v", st)
	}
}

func TestCancelDoesNotError(t *testing.T) {
	s := start(t, "")
	_ = s.Send(context.Background(), core.UserInput{Text: "x"})
	collect(t, s, core.EvTurnDone, nil)
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
}

func TestCommandsFromSkillsList(t *testing.T) {
	cmds, err := codex.New("testdata/fakecodex.py").Commands(context.Background())
	if err != nil || len(cmds) != 1 || cmds[0].Name != "demo-skill" || cmds[0].Description != "short" {
		t.Fatalf("cmds=%+v err=%v", cmds, err)
	}
}

func TestSummarizeUsesCheapestModelReadOnly(t *testing.T) {
	log := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("FAKECODEX_LOG", log)
	abs, _ := filepath.Abs("testdata/fakecodex.py")
	e := codex.New(abs)
	if m := e.SummaryModel(); m != "fake-mini" {
		t.Fatalf("cheapest=%q", m)
	}
	text, usage, err := e.Summarize(context.Background(), "sys", "prompt")
	if err != nil || text != "codex-summary" || usage.InputTokens != 9 {
		t.Fatalf("text=%q usage=%+v err=%v", text, usage, err)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "--sandbox read-only") || !strings.Contains(string(b), "--model fake-mini") {
		t.Fatalf("args:\n%s", b)
	}
}

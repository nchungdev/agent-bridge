package agy_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nchungdev/agent-hub/internal/adapters/agy"
	"github.com/nchungdev/agent-hub/internal/core"
)

func start(t *testing.T, o core.StartOpts) core.Session {
	s, err := agy.New("testdata/fakeagy.py").Start(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func collect(t *testing.T, s core.Session, until core.EventType) []core.Event {
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
			if ev.Type == until {
				return out
			}
		case <-timeout:
			t.Fatalf("timeout: %+v", out)
		}
	}
}

func TestTextNoDuplicateAndUsage(t *testing.T) {
	s := start(t, core.StartOpts{Mode: "accept-edits"})
	if !strings.HasPrefix(s.EngineSessionID(), "conv-") {
		t.Fatalf("id=%q", s.EngineSessionID())
	}
	_ = s.Send(context.Background(), core.UserInput{Text: "hi"})
	evs := collect(t, s, core.EvTurnDone)
	text := ""
	for _, e := range evs {
		if e.Type == core.EvTextDelta {
			text += e.Text
		}
	}
	if text != "hello " {
		t.Fatalf("text=%q", text)
	}
	if u := evs[len(evs)-1].Usage; u == nil || u.OutputTokens != 4 {
		t.Fatalf("usage=%+v", u)
	}
	// second turn on the same long-lived process
	_ = s.Send(context.Background(), core.UserInput{Text: "again"})
	collect(t, s, core.EvTurnDone)
}

func TestFinalResponseUsedWhenNoDeltas(t *testing.T) {
	s := start(t, core.StartOpts{})
	_ = s.Send(context.Background(), core.UserInput{Text: "[nodelta]"})
	evs := collect(t, s, core.EvTurnDone)
	if evs[0].Type != core.EvTextDelta || evs[0].Text != "final only" {
		t.Fatalf("%+v", evs)
	}
}

func TestQuotaClassified(t *testing.T) {
	s := start(t, core.StartOpts{})
	_ = s.Send(context.Background(), core.UserInput{Text: "[quota]"})
	evs := collect(t, s, core.EvError)
	if evs[len(evs)-1].Err.Kind != "quota_exceeded" {
		t.Fatalf("%+v", evs[len(evs)-1].Err)
	}
}

func TestCancelRespawnsOnSameConversation(t *testing.T) {
	log := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("FAKEAGY_LOG", log)
	s := start(t, core.StartOpts{Mode: "plan"})
	id := s.EngineSessionID()
	_ = s.Send(context.Background(), core.UserInput{Text: "[slow]"})
	time.Sleep(100 * time.Millisecond)
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	evs := collect(t, s, core.EvTurnDone)
	if evs[0].Err == nil || evs[0].Err.Kind != "cancelled" {
		t.Fatalf("%+v", evs)
	}
	if s.EngineSessionID() != id {
		t.Fatalf("conversation changed: %s -> %s", id, s.EngineSessionID())
	}
	_ = s.Send(context.Background(), core.UserInput{Text: "after"})
	collect(t, s, core.EvTurnDone)
	b, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || strings.Contains(lines[0], "--conversation") || !strings.Contains(lines[1], "--conversation "+id) {
		t.Fatalf("spawn args:\n%s", b)
	}
	if !strings.Contains(lines[0], "--mode plan") {
		t.Fatalf("plan mode not passed: %s", lines[0])
	}
}

func TestBypassFlagOnlyWhenRequested(t *testing.T) {
	log := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("FAKEAGY_LOG", log)
	start(t, core.StartOpts{Mode: "accept-edits"})
	start(t, core.StartOpts{Mode: "bypass"})
	b, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || strings.Contains(lines[0], "dangerously") || !strings.Contains(lines[1], "--dangerously-skip-permissions") {
		t.Fatalf("args:\n%s", b)
	}
}

func TestResumeAndModels(t *testing.T) {
	s := start(t, core.StartOpts{ResumeID: "abc"})
	if s.EngineSessionID() != "abc" {
		t.Fatalf("id=%s", s.EngineSessionID())
	}
	ms, _ := agy.New("testdata/fakeagy.py").Models(context.Background())
	if len(ms) != 1 || ms[0].ID != "m-high" {
		t.Fatalf("%+v", ms)
	}
	if err := s.Decide("x", core.Decision{}); err == nil {
		t.Fatal("agy has no approval channel")
	}
}

func TestSkillCommandsFromDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".gemini", "skills", "film-oracle")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: film-oracle\ndescription: \"Find films\"\n---\n# body\n"), 0o644)
	cmds, err := agy.New("x").Commands(context.Background())
	if err != nil || len(cmds) != 1 || cmds[0].Name != "film-oracle" || cmds[0].Description != "Find films" {
		t.Fatalf("cmds=%+v err=%v", cmds, err)
	}
}

func TestEffortFlagSkippedForModelsWithEffortInTheName(t *testing.T) {
	log := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("FAKEAGY_LOG", log)
	start(t, core.StartOpts{Model: "gemini-3.8-flash-high", Effort: "medium"})
	start(t, core.StartOpts{Model: "gemini-3.8-flash", Effort: "medium"})
	b, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || strings.Contains(lines[0], "--effort") || !strings.Contains(lines[1], "--effort medium") {
		t.Fatalf("args:\n%s", b)
	}
}

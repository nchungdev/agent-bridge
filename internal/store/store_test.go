package store_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/nchungdev/agent-hub/internal/core"
	"github.com/nchungdev/agent-hub/internal/store"
	_ "modernc.org/sqlite"
)

func newStore(t *testing.T) *store.Store {
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRedact(t *testing.T) {
	in := `key sk-abcdefghijklmnopqrstuv and ghp_abcdefghijklmnopqrstuvwx and Authorization: Bearer abcdefghijklmnopqrst and password=hunter2xyz`
	out := store.Redact(in)
	for _, bad := range []string{"sk-abcdef", "ghp_abcdef", "abcdefghijklmnopqrst", "hunter2xyz"} {
		if strings.Contains(out, bad) {
			t.Fatalf("leaked %q in %q", bad, out)
		}
	}
}

func TestAppendSinceTail(t *testing.T) {
	s := newStore(t)
	for i := 0; i < 5; i++ {
		if _, err := s.Append(core.Event{ConvID: "c", Type: core.EvTextDelta, Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Append(core.Event{ConvID: "other", Type: core.EvTextDelta}); err != nil {
		t.Fatal(err)
	}
	since, _ := s.Since("c", 2, 0)
	if len(since) != 3 || since[0].Seq != 3 {
		t.Fatalf("since: %+v", since)
	}
	tail, _ := s.Tail("c", 2)
	if len(tail) != 2 || tail[1].Seq != 5 {
		t.Fatalf("tail: %+v", tail)
	}
}

func TestWorkingStateByRule(t *testing.T) {
	s := newStore(t)
	_ = s.ApplyEvent(core.Event{ConvID: "c", Type: core.EvDiff, Diff: &core.Diff{File: "a.go"}})
	_ = s.ApplyEvent(core.Event{ConvID: "c", Type: core.EvDiff, Diff: &core.Diff{File: "a.go"}})
	_ = s.ApplyEvent(core.Event{ConvID: "c", Type: core.EvError, Err: &core.ErrInfo{Message: "boom"}})
	ws, _ := s.GetState("c")
	if len(ws.FilesChanged) != 1 || ws.LastError != "boom" {
		t.Fatalf("ws=%+v", ws)
	}
}

func TestApprovalLifecycle(t *testing.T) {
	s := newStore(t)
	_ = s.AddApproval("c", "e", core.ApprovalRequest{ID: "1", Tool: "Bash", Args: map[string]any{"command": "ls"}})
	p, _ := s.PendingApprovals("c")
	if len(p) != 1 {
		t.Fatal("expected pending")
	}
	if err := s.ResolveApproval("1", core.Decision{Allow: false}); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveApproval("1", core.Decision{Allow: true}); err == nil {
		t.Fatal("double resolve must fail")
	}
}

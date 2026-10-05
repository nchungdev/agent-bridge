package store_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/nchungdev/agent-bridge/internal/core"
	"github.com/nchungdev/agent-bridge/internal/store"
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

func TestMetaForkAndDelete(t *testing.T) {
	s := newStore(t)
	for _, e := range []core.Event{
		{ConvID: "a", Type: core.EvUserMessage, Text: "hi"},
		{ConvID: "a", Type: core.EvApprovalRequest, Approval: &core.ApprovalRequest{ID: "x", Tool: "Bash"}},
		{ConvID: "a", Type: core.EvTextDelta, Text: "yo", Engine: "e"},
	} {
		if _, err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	yes, grp := true, "work"
	_ = s.UpdateMeta("a", store.MetaPatch{Pinned: &yes, Group: &grp})
	_ = s.UpdateMeta("a", store.MetaPatch{Unread: &yes}) // partial patch must not clear the rest
	m, _ := s.ListMeta()
	if !m["a"].Pinned || m["a"].Group != "work" || !m["a"].Unread || m["a"].Archived {
		t.Fatalf("meta=%+v", m["a"])
	}
	n, err := s.ForkEvents("a", "b")
	if err != nil || n != 2 {
		t.Fatalf("fork n=%d err=%v", n, err)
	}
	evs, _ := s.Since("b", 0, 10)
	if len(evs) != 2 || evs[0].ConvID != "b" || evs[0].Seq != 1 || evs[1].Seq != 2 || evs[1].Text != "yo" {
		t.Fatalf("forked=%+v", evs)
	}
	if err := s.DeleteConv("a"); err != nil {
		t.Fatal(err)
	}
	left, _ := s.Since("a", 0, 10)
	m, _ = s.ListMeta()
	if len(left) != 0 || len(m) != 0 {
		t.Fatalf("not deleted: events=%d meta=%v", len(left), m)
	}
	if evs, _ := s.Since("b", 0, 10); len(evs) != 2 {
		t.Fatal("delete must not touch the fork")
	}
}

func TestMetaTitleOverlayForReadOnlyItems(t *testing.T) {
	s := newStore(t)
	title, yes := "Better name", true
	_ = s.UpdateMeta("agy-123", store.MetaPatch{Title: &title, Archived: &yes})
	m, _ := s.ListMeta()
	if m["agy-123"].Title != "Better name" || !m["agy-123"].Archived {
		t.Fatalf("meta=%+v", m["agy-123"])
	}
}

func TestRedactionNeverCorruptsStoredJSON(t *testing.T) {
	s := newStore(t)
	// code that used to break the JSON: a secret-looking assignment followed by quotes and brackets
	cmd := "access_token = cp[\"gdrive\"][\"token\"]\nprint(\"token = abcdefghijkl\")\npassword=hunter2xyz\\\""
	out := "157:   token = abcdefghijklmnop\"  cookie_hdr = self"
	if _, err := s.Append(core.Event{ConvID: "c", Type: core.EvToolCall, Tool: &core.ToolCall{ID: "1", Name: "Bash", Args: map[string]any{"command": cmd}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(core.Event{ConvID: "c", Type: core.EvToolResult, Tool: &core.ToolCall{ID: "1", Output: out}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(core.Event{ConvID: "c", Type: core.EvTextDelta, Text: "ok"}); err != nil {
		t.Fatal(err)
	}
	evs, err := s.Since("c", 0, 100)
	if err != nil || len(evs) != 3 {
		t.Fatalf("events=%d err=%v", len(evs), err)
	}
	if got := evs[0].Tool.Args["command"].(string); strings.Contains(got, "hunter2xyz") || strings.Contains(got, "abcdefghijkl") || !strings.Contains(got, `cp["gdrive"]`) {
		t.Fatalf("redaction wrong: %q", got)
	}
	if strings.Contains(evs[1].Tool.Output, "abcdefghijklmnop") {
		t.Fatalf("secret leaked: %q", evs[1].Tool.Output)
	}
	// working state with the same kind of text stays decodable
	if err := s.SetState("c", store.WorkingState{Summary: "uses token = abcdefghijkl and \"quotes\" \\", LastError: `password="hunter2xyz"`}); err != nil {
		t.Fatal(err)
	}
	ws, err := s.GetState("c")
	if err != nil || strings.Contains(ws.Summary, "abcdefghijkl") || strings.Contains(ws.LastError, "hunter2xyz") {
		t.Fatalf("ws=%+v err=%v", ws, err)
	}
}

func TestDamagedRowDoesNotHideTheConversationAndIsRepaired(t *testing.T) {
	db, _ := sql.Open("sqlite", "file:damaged?mode=memory&cache=shared")
	db.SetMaxOpenConns(1)
	defer db.Close()
	s, _ := store.New(db)
	for i := 0; i < 3; i++ {
		_, _ = s.Append(core.Event{ConvID: "c", Type: core.EvTextDelta, Text: "row"})
	}
	// damage the middle row the way the old redaction did
	if _, err := db.Exec(`UPDATE v2_events SET payload='{"type":"tool_call","tool":{"args":{"command":"x = [REDACTED]"y"}}' WHERE conv_id='c' AND seq=2`); err != nil {
		t.Fatal(err)
	}
	evs, err := s.Since("c", 0, 10)
	if err != nil || len(evs) != 3 || evs[0].Text != "row" || evs[2].Text != "row" || evs[1].Type != core.EvStateChange {
		t.Fatalf("a damaged row must not hide the others: %+v err=%v", evs, err)
	}
	n, err := s.RepairEvents()
	if err != nil || n != 1 {
		t.Fatalf("repaired=%d err=%v", n, err)
	}
	if n, _ := s.RepairEvents(); n != 0 {
		t.Fatalf("second repair should find nothing, got %d", n)
	}
}

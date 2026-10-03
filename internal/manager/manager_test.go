package manager_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nchungdev/agent-hub/internal/core"
	"github.com/nchungdev/agent-hub/internal/enginetest"
	"github.com/nchungdev/agent-hub/internal/manager"
	"github.com/nchungdev/agent-hub/internal/store"
	_ "modernc.org/sqlite"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_")))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	st, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func wait(t *testing.T, ch <-chan core.Event, typ core.EventType) core.Event {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed waiting for %s", typ)
			}
			if ev.Type == typ {
				return ev
			}
		case <-timeout:
			t.Fatalf("timeout waiting for %s", typ)
		}
	}
}

func waitState(t *testing.T, m *manager.Manager, conv, eng string, want core.State) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if m.State(conv, eng) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("state = %q, want %q", m.State(conv, eng), want)
}

func TestSendAndTurnDone(t *testing.T) {
	st := newStore(t)
	m := manager.New(st, []core.Engine{enginetest.New("a")}, manager.Config{})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	if err := m.Send(context.Background(), "c1", "a", core.UserInput{Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	wait(t, ch, core.EvTurnDone)
	waitState(t, m, "c1", "a", core.StateIdle)
	evs, _ := st.Tail("c1", 50)
	var txt strings.Builder
	var seqOK = true
	var prev int64
	for _, e := range evs {
		if e.Type == core.EvTextDelta {
			txt.WriteString(e.Text)
		}
		if e.Seq <= prev {
			seqOK = false
		}
		prev = e.Seq
	}
	if !strings.Contains(txt.String(), "echo: hello") || !seqOK {
		t.Fatalf("persisted text=%q seqOK=%v", txt.String(), seqOK)
	}
}

func TestApprovalAskThenAllow(t *testing.T) {
	st := newStore(t)
	m := manager.New(st, []core.Engine{enginetest.New("a")}, manager.Config{})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "do [approve]"})
	ev := wait(t, ch, core.EvApprovalRequest)
	waitState(t, m, "c1", "a", core.StateAwaitingApproval)
	pend, _ := st.PendingApprovals("c1")
	if len(pend) != 1 {
		t.Fatalf("pending = %d", len(pend))
	}
	if err := m.Decide("c1", ev.Approval.ID, core.Decision{Allow: true}); err != nil {
		t.Fatal(err)
	}
	wait(t, ch, core.EvTurnDone)
	pend, _ = st.PendingApprovals("c1")
	if len(pend) != 0 {
		t.Fatalf("pending after decide = %d", len(pend))
	}
	if err := m.Decide("c1", ev.Approval.ID, core.Decision{Allow: true}); err == nil {
		t.Fatal("second decide should fail")
	}
}

func TestPolicyPlanModeDenies(t *testing.T) {
	st := newStore(t)
	m := manager.New(st, []core.Engine{enginetest.New("a")}, manager.Config{})
	_ = m.SetConv(store.ConvSettings{ConvID: "c1", Mode: "plan"})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "[approve]"})
	r := wait(t, ch, core.EvApprovalResolved)
	if r.Data["allow"] != false || r.Data["by"] != "policy" {
		t.Fatalf("data = %v", r.Data)
	}
	wait(t, ch, core.EvTurnDone)
}

func TestBypassModeAllows(t *testing.T) {
	st := newStore(t)
	m := manager.New(st, []core.Engine{enginetest.New("a")}, manager.Config{})
	_ = m.SetConv(store.ConvSettings{ConvID: "c1", Mode: "bypass"})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "[approve]"})
	r := wait(t, ch, core.EvApprovalResolved)
	if r.Data["allow"] != true {
		t.Fatalf("data = %v", r.Data)
	}
	wait(t, ch, core.EvTurnDone)
}

func TestQueueRunsInOrder(t *testing.T) {
	st := newStore(t)
	m := manager.New(st, []core.Engine{enginetest.New("a")}, manager.Config{})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "first [slow]"})
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "second"})
	wait(t, ch, core.EvTurnDone)
	wait(t, ch, core.EvTurnDone)
	evs, _ := st.Tail("c1", 100)
	var users []string
	for _, e := range evs {
		if e.Type == core.EvUserMessage {
			users = append(users, e.Text)
		}
	}
	if len(users) != 2 || users[0] != "first [slow]" || users[1] != "second" {
		t.Fatalf("users = %v", users)
	}
}

func TestCancelClearsQueueKeepsProcess(t *testing.T) {
	st := newStore(t)
	eng := enginetest.New("a")
	m := manager.New(st, []core.Engine{eng}, manager.Config{})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "x [slow]"})
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "queued"})
	_ = m.Cancel("c1")
	wait(t, ch, core.EvTurnDone)
	waitState(t, m, "c1", "a", core.StateIdle)
	if eng.Starts() != 1 {
		t.Fatalf("process restarted: starts=%d", eng.Starts())
	}
}

func TestPoolEvictsLRUAndResumes(t *testing.T) {
	st := newStore(t)
	eng := enginetest.New("a")
	m := manager.New(st, []core.Engine{eng}, manager.Config{MaxLive: 1})
	chA, cA := m.Subscribe("A")
	defer cA()
	_ = m.Send(context.Background(), "A", "a", core.UserInput{Text: "one"})
	wait(t, chA, core.EvTurnDone)
	waitState(t, m, "A", "a", core.StateIdle)
	b, _ := st.GetBinding("A", "a")
	firstID := b.EngineSessionID

	chB, cB := m.Subscribe("B")
	defer cB()
	_ = m.Send(context.Background(), "B", "a", core.UserInput{Text: "two"})
	wait(t, chB, core.EvTurnDone)
	if m.LiveCount() != 1 {
		t.Fatalf("live = %d", m.LiveCount())
	}
	b, _ = st.GetBinding("A", "a")
	if b.State != core.StateSuspended {
		t.Fatalf("A state = %s", b.State)
	}
	waitState(t, m, "B", "a", core.StateIdle)
	_ = m.Send(context.Background(), "A", "a", core.UserInput{Text: "three"})
	wait(t, chA, core.EvTurnDone)
	if eng.LastStart.ResumeID != firstID || firstID == "" {
		t.Fatalf("resume id = %q want %q", eng.LastStart.ResumeID, firstID)
	}
}

func TestPoolFullWhenNoneIdle(t *testing.T) {
	st := newStore(t)
	m := manager.New(st, []core.Engine{enginetest.New("a")}, manager.Config{MaxLive: 1})
	_ = m.Send(context.Background(), "A", "a", core.UserInput{Text: "x [approve]"})
	waitState(t, m, "A", "a", core.StateAwaitingApproval)
	if err := m.Send(context.Background(), "B", "a", core.UserInput{Text: "y"}); err == nil {
		t.Fatal("expected pool-full error")
	}
}

func TestIdleReap(t *testing.T) {
	st := newStore(t)
	m := manager.New(st, []core.Engine{enginetest.New("a")}, manager.Config{IdleTimeout: time.Minute})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "hi"})
	wait(t, ch, core.EvTurnDone)
	waitState(t, m, "c1", "a", core.StateIdle)
	if n := m.ReapIdle(time.Now().Add(30 * time.Second)); n != 0 {
		t.Fatalf("reaped early: %d", n)
	}
	if n := m.ReapIdle(time.Now().Add(2 * time.Minute)); n != 1 {
		t.Fatalf("reaped = %d", n)
	}
	if m.LiveCount() != 0 {
		t.Fatal("still live")
	}
}

func TestRecoverResetsBindings(t *testing.T) {
	st := newStore(t)
	_ = st.UpsertBinding(store.Binding{ConvID: "c", Engine: "a", State: core.StateRunning, EngineSessionID: "s"})
	m := manager.New(st, []core.Engine{enginetest.New("a")}, manager.Config{})
	n, err := m.Recover()
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	b, _ := st.GetBinding("c", "a")
	if b.State != core.StateSuspended {
		t.Fatalf("state=%s", b.State)
	}
}

func TestCrashMarksFailedAndRestarts(t *testing.T) {
	st := newStore(t)
	eng := enginetest.New("a")
	m := manager.New(st, []core.Engine{eng}, manager.Config{})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "[crash]"})
	e := wait(t, ch, core.EvError)
	if e.Err.Kind != "crashed" {
		t.Fatalf("kind=%s", e.Err.Kind)
	}
	for i := 0; i < 100 && m.LiveCount() != 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "again"})
	wait(t, ch, core.EvTurnDone)
	if eng.Starts() != 2 {
		t.Fatalf("starts=%d", eng.Starts())
	}
}

func TestSwitchEngineHandoff(t *testing.T) {
	st := newStore(t)
	a, b := enginetest.New("a"), enginetest.New("b")
	m := manager.New(st, []core.Engine{a, b}, manager.Config{MaxLive: 2})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "build the parser"})
	wait(t, ch, core.EvTurnDone)
	waitState(t, m, "c1", "a", core.StateIdle)
	_ = m.SetConv(store.ConvSettings{ConvID: "c1", ActiveEngine: "a"})
	if err := m.SwitchEngine("c1", "b"); err != nil {
		t.Fatal(err)
	}
	_ = m.Send(context.Background(), "c1", "b", core.UserInput{Text: "continue"})
	wait(t, ch, core.EvTurnDone)
	pre := b.Preamble()
	if !strings.Contains(pre, "build the parser") || !strings.Contains(pre, "[a]") {
		t.Fatalf("preamble missing context: %q", pre)
	}
	// switching back: a only receives what it has not seen (b's turn)
	waitState(t, m, "c1", "b", core.StateIdle)
	if err := m.SwitchEngine("c1", "a"); err != nil {
		t.Fatal(err)
	}
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "and now?"})
	wait(t, ch, core.EvTurnDone)
	if p := a.Preamble(); !strings.Contains(p, "[b]") || strings.Contains(p, "[a]") {
		t.Fatalf("catch-up preamble wrong: %q", p)
	}
}

func TestSwitchRefusedWhileRunning(t *testing.T) {
	st := newStore(t)
	m := manager.New(st, []core.Engine{enginetest.New("a"), enginetest.New("b")}, manager.Config{MaxLive: 2})
	_ = m.SetConv(store.ConvSettings{ConvID: "c1", ActiveEngine: "a"})
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "[slow]"})
	waitState(t, m, "c1", "a", core.StateRunning)
	if err := m.SwitchEngine("c1", "b"); err == nil {
		t.Fatal("switch should be refused while running")
	}
}

func TestUnknownEngine(t *testing.T) {
	m := manager.New(newStore(t), nil, manager.Config{})
	if err := m.Send(context.Background(), "c", "nope", core.UserInput{Text: "x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestFatalErrorFreesSlotAndRestarts(t *testing.T) {
	st := newStore(t)
	eng := enginetest.New("a")
	m := manager.New(st, []core.Engine{eng}, manager.Config{MaxLive: 1})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "[quota]"})
	e := wait(t, ch, core.EvError)
	if e.Err.Kind != "quota_exceeded" {
		t.Fatalf("kind=%s", e.Err.Kind)
	}
	for i := 0; i < 200 && m.LiveCount() != 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if m.LiveCount() != 0 {
		t.Fatal("failed binding still holds a pool slot")
	}
	// a different conversation can now use the single slot, and c1 can retry
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "retry"})
	wait(t, ch, core.EvTurnDone)
	if eng.Starts() != 2 {
		t.Fatalf("starts=%d", eng.Starts())
	}
}

type gatedEngine struct {
	*enginetest.Engine
	st core.AuthStatus
}

func (g gatedEngine) Status(context.Context) core.AuthStatus { return g.st }

func TestNotLoggedInEngineIsNotSpawned(t *testing.T) {
	inner := enginetest.New("a")
	g := gatedEngine{Engine: inner, st: core.AuthStatus{Installed: true, Known: true, LoggedIn: false, Detail: "not logged in", LoginHint: "run x login"}}
	m := manager.New(newStore(t), []core.Engine{g}, manager.Config{})
	err := m.Send(context.Background(), "c1", "a", core.UserInput{Text: "hi"})
	if err == nil || !strings.Contains(err.Error(), "run x login") {
		t.Fatalf("err=%v", err)
	}
	if inner.Starts() != 0 || m.LiveCount() != 0 {
		t.Fatal("process was spawned for an unauthenticated engine")
	}
	// unknown status (CLI cannot tell) must still be allowed to try
	g.st = core.AuthStatus{Installed: true, Known: false}
	m2 := manager.New(newStore(t), []core.Engine{g}, manager.Config{})
	if err := m2.Send(context.Background(), "c1", "a", core.UserInput{Text: "hi"}); err != nil {
		t.Fatal(err)
	}
}

func TestAllowForSessionAutoApprovesSameTool(t *testing.T) {
	st := newStore(t)
	m := manager.New(st, []core.Engine{enginetest.New("a")}, manager.Config{})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "one [approve]"})
	ev := wait(t, ch, core.EvApprovalRequest)
	if err := m.Decide("c1", ev.Approval.ID, core.Decision{Allow: true, Scope: "session"}); err != nil {
		t.Fatal(err)
	}
	wait(t, ch, core.EvTurnDone)
	waitState(t, m, "c1", "a", core.StateIdle)
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "two [approve]"})
	deadline := time.After(3 * time.Second)
	for found := false; !found; {
		select {
		case ev := <-ch:
			if ev.Type == core.EvApprovalResolved && ev.Data["by"] == "policy" && ev.Data["allow"] == true {
				found = true
			}
		case <-deadline:
			t.Fatal("second approval was not auto-allowed by policy")
		}
	}
	wait(t, ch, core.EvTurnDone)
}

type restartEngine struct{ *enginetest.Engine }

func (restartEngine) Capabilities() core.Capabilities {
	return core.Capabilities{ModeRequiresRestart: true, Resume: true}
}

func TestModeChangeRestartsIdleSession(t *testing.T) {
	st := newStore(t)
	eng := restartEngine{enginetest.New("a")}
	m := manager.New(st, []core.Engine{eng}, manager.Config{})
	ch, cancel := m.Subscribe("c1")
	defer cancel()
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "hi"})
	wait(t, ch, core.EvTurnDone)
	waitState(t, m, "c1", "a", core.StateIdle)
	_ = m.SetConv(store.ConvSettings{ConvID: "c1", Mode: "plan"})
	if m.LiveCount() != 0 {
		t.Fatalf("live=%d, want suspended", m.LiveCount())
	}
	_ = m.Send(context.Background(), "c1", "a", core.UserInput{Text: "again"})
	wait(t, ch, core.EvTurnDone)
	if eng.LastStart.Mode != "plan" || eng.LastStart.ResumeID == "" {
		t.Fatalf("restart opts=%+v", eng.LastStart)
	}
}

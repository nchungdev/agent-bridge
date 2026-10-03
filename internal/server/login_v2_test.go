package server

import (
	"encoding/json"
	"context"
	"database/sql"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nchungdev/agent-hub/internal/core"
	"github.com/nchungdev/agent-hub/internal/enginetest"
	"github.com/nchungdev/agent-hub/internal/manager"
	"github.com/nchungdev/agent-hub/internal/store"
	_ "modernc.org/sqlite"
)

type loginEngine struct {
	*enginetest.Engine
	bin string
}

func (l loginEngine) LoginCommand() (string, []string) { return l.bin, nil }

func fakeLogin(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "fakelogin.sh")
	script := "#!/bin/sh\nprintf '\\033[94mOpen https://auth.example.com/device\\033[0m\\nEnter code ABCD-EF12\\nPaste code here > '\nread code\n[ \"$code\" = \"good\" ] && echo OK && exit 0\nexit 3\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func waitFinished(t *testing.T, v *V2, id string) loginState {
	for i := 0; i < 100; i++ {
		if st := v.getLogin(id).snapshot(); st.Finished {
			return st
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("login did not finish")
	return loginState{}
}

func TestLoginFlowSuccess(t *testing.T) {
	v := &V2{Engines: []core.Engine{loginEngine{enginetest.New("x"), fakeLogin(t)}}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v2/engines/x/login", nil)
	r.SetPathValue("id", "x")
	v.handleLoginStart(w, r)
	var st loginState
	if err := jsonDecode(w.Body.String(), &st); err != nil {
		t.Fatal(err)
	}
	if len(st.URLs) != 1 || st.URLs[0] != "https://auth.example.com/device" || st.Code != "ABCD-EF12" || !st.NeedsCode || !st.Running {
		t.Fatalf("state=%+v", st)
	}
	if strings.Contains(st.Output, "\x1b") {
		t.Fatal("ANSI not stripped")
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/api/v2/engines/x/login/input", strings.NewReader(`{"text":"good"}`))
	r.SetPathValue("id", "x")
	v.handleLoginInput(w, r)
	if fin := waitFinished(t, v, "x"); !fin.Success {
		t.Fatalf("state=%+v", fin)
	}
}

func TestLoginFlowFailureAndCancel(t *testing.T) {
	v := &V2{Engines: []core.Engine{loginEngine{enginetest.New("x"), fakeLogin(t)}}}
	if _, err := v.startLogin("x"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	f := v.getLogin("x")
	_, _ = f.stdin.Write([]byte("bad\n"))
	if fin := waitFinished(t, v, "x"); fin.Success || fin.Error == "" {
		t.Fatalf("state=%+v", fin)
	}
	f2, _ := v.startLogin("x")
	time.Sleep(300 * time.Millisecond)
	f2.cancel()
	if fin := waitFinished(t, v, "x"); fin.Success {
		t.Fatal("cancelled login must not succeed")
	}
	if _, err := v.startLogin("nope"); err == nil {
		t.Fatal("unknown engine must error")
	}
}

type countingEngine struct {
	*enginetest.Engine
	calls *int
	n     int
}

func (c countingEngine) Models(context.Context) ([]core.Model, error) {
	*c.calls++
	ms := make([]core.Model, c.n)
	for i := range ms {
		ms[i] = core.Model{ID: "m", Name: "m"}
	}
	return ms, nil
}
func (c countingEngine) Capabilities() core.Capabilities { return core.Capabilities{ModelListing: true} }

func TestModelCacheTTLPersistAndRefresh(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	eng := countingEngine{enginetest.New("x"), &calls, 3}
	v := &V2{Engines: []core.Engine{eng}, DataDir: dir}
	ctx := context.Background()
	v.cachedModels(ctx, eng, false)
	v.cachedModels(ctx, eng, false)
	if calls != 1 {
		t.Fatalf("second read must hit cache, calls=%d", calls)
	}
	// persisted: a fresh process reads the file instead of fetching
	v2 := &V2{Engines: []core.Engine{eng}, DataDir: dir}
	v2.cachedModels(ctx, eng, false)
	if calls != 1 {
		t.Fatalf("restart must not refetch, calls=%d", calls)
	}
	v2.cachedModels(ctx, eng, true)
	if calls != 2 {
		t.Fatalf("force must refetch, calls=%d", calls)
	}
	// expiry after 24h
	v2.modelMu.Lock()
	c := v2.modelCache["x"]
	c.FetchedAt = time.Now().Add(-25 * time.Hour)
	v2.modelCache["x"] = c
	v2.modelMu.Unlock()
	v2.cachedModels(ctx, eng, false)
	if calls != 3 {
		t.Fatalf("expired entry must refetch, calls=%d", calls)
	}
	// a failed (fallback-only) listing is retried soon, not cached for a day
	bad := countingEngine{enginetest.New("y"), &calls, 1}
	got := v2.cachedModels(ctx, bad, false)
	if time.Since(got.FetchedAt) < 23*time.Hour {
		t.Fatalf("fallback result should expire quickly: %v", got.FetchedAt)
	}
}

func TestShellCommandRecordedAndSurfacedToNextTurn(t *testing.T) {
	db, err := sql.Open("sqlite", "file:shelltest?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	st, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	eng := enginetest.New("a")
	mgr := manager.New(st, []core.Engine{eng}, manager.Config{})
	v := &V2{Mgr: mgr, Store: st, Engines: []core.Engine{eng}}
	v.runShell("c1", "echo hello-from-shell; echo oops 1>&2; exit 3", t.TempDir())
	evs, _ := st.Tail("c1", 5)
	if len(evs) != 1 || evs[0].Type != core.EvShell {
		t.Fatalf("events=%+v", evs)
	}
	out := evs[0].Tool.Output
	if !strings.Contains(out, "hello-from-shell") || !strings.Contains(out, "oops") || evs[0].Data["exit_code"] != float64(3) {
		t.Fatalf("out=%q data=%v", out, evs[0].Data)
	}
	// the next model turn is told about it
	ch, cancel := mgr.Subscribe("c1")
	defer cancel()
	_ = mgr.Send(context.Background(), "c1", "a", core.UserInput{Text: "what did that print?"})
	for ev := range ch {
		if ev.Type == core.EvTurnDone {
			break
		}
	}
	if pre := eng.Preamble(); !strings.Contains(pre, "hello-from-shell") || !strings.Contains(pre, "exit code 3") {
		t.Fatalf("preamble=%q", pre)
	}
	// and not repeated on the turn after
	_ = mgr.Send(context.Background(), "c1", "a", core.UserInput{Text: "again"})
	time.Sleep(300 * time.Millisecond)
	if strings.Contains(eng.Preamble(), "hello-from-shell") {
		t.Fatal("shell context repeated")
	}
}

func TestHistoryIsNeverNull(t *testing.T) {
	db, _ := sql.Open("sqlite", "file:histnull?mode=memory&cache=shared")
	db.SetMaxOpenConns(1)
	defer db.Close()
	st, _ := store.New(db)
	v := &V2{Store: st}
	b, _ := json.Marshal(v.history("empty-conv", 0))
	if string(b) != "[]" {
		t.Fatalf("history JSON=%s, want []", b)
	}
}

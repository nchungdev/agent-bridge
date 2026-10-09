package zalo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nchungdev/agent-bridge/internal/core"
	"github.com/nchungdev/agent-bridge/internal/store"
)

type fakeAgent struct {
	mu       sync.Mutex
	ch       chan core.Event
	sent     []string
	modes    []string
	decided  []core.Decision
	canceled int
	script   func(a *fakeAgent) // events emitted after Send
}

func (a *fakeAgent) Subscribe(string) (<-chan core.Event, func()) { return a.ch, func() {} }
func (a *fakeAgent) Send(_ context.Context, _, _ string, in core.UserInput) error {
	a.mu.Lock()
	a.sent = append(a.sent, in.Text)
	a.mu.Unlock()
	if a.script != nil {
		go a.script(a)
	}
	return nil
}
func (a *fakeAgent) SetConv(c store.ConvSettings) error {
	a.mu.Lock()
	a.modes = append(a.modes, c.Mode)
	a.mu.Unlock()
	return nil
}
func (a *fakeAgent) Decide(_, _ string, d core.Decision) error {
	a.mu.Lock()
	a.decided = append(a.decided, d)
	a.mu.Unlock()
	return nil
}
func (a *fakeAgent) Cancel(string) error { a.mu.Lock(); a.canceled++; a.mu.Unlock(); return nil }

type memSettings struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSettings) GetSetting(k string) string { s.mu.Lock(); defer s.mu.Unlock(); return s.m[k] }
func (s *memSettings) SetSetting(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}

type recorder struct {
	mu   sync.Mutex
	sent []string
}

func (r *recorder) SendText(_ context.Context, _, text string) error {
	r.mu.Lock()
	r.sent = append(r.sent, text)
	r.mu.Unlock()
	return nil
}
func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent...)
}

func newSvc(t *testing.T) (*Service, *fakeAgent, *recorder) {
	t.Helper()
	cfg := Config{Token: "tok", Secret: "supersecret", Allowed: map[string]bool{"u1": true}, Engine: "claude", Mode: "plan", Workspace: "/ws"}
	a := &fakeAgent{ch: make(chan core.Event, 16)}
	rec := &recorder{}
	s := New(cfg, a, &memSettings{m: map[string]string{}}, rec,
		func(string, string) (string, error) { return "conv-1", nil }, func(string) bool { return true })
	return s, a, rec
}

func msg(id, text string) Message {
	m := Message{MessageID: id, Text: text}
	m.From.ID = "u1"
	m.Chat.ID, m.Chat.Type = "c1", "PRIVATE"
	return m
}

func TestTurnSendsAgentAnswerAndUsesReadOnlyMode(t *testing.T) {
	s, a, rec := newSvc(t)
	a.script = func(a *fakeAgent) {
		a.ch <- core.Event{Type: core.EvTextDelta, Text: "xin "}
		a.ch <- core.Event{Type: core.EvTextDelta, Text: "chào"}
		a.ch <- core.Event{Type: core.EvTurnDone}
	}
	s.Handle(context.Background(), msg("m1", "hello"))
	if got := rec.all(); len(got) != 1 || got[0] != "xin chào" {
		t.Fatalf("replies = %q", got)
	}
	if len(a.modes) != 1 || a.modes[0] != "plan" {
		t.Fatalf("conversation mode = %v, want [plan]", a.modes)
	}
}

func TestStrangerIsIgnored(t *testing.T) {
	s, a, rec := newSvc(t)
	m := msg("m1", "hello")
	m.From.ID = "intruder"
	s.Handle(context.Background(), m)
	if len(a.sent) != 0 || len(rec.all()) != 0 {
		t.Fatalf("stranger got a response: agent=%v zalo=%v", a.sent, rec.all())
	}
}

func TestGroupChatIsIgnored(t *testing.T) {
	s, a, _ := newSvc(t)
	m := msg("m1", "hello")
	m.Chat.Type = "GROUP"
	s.Handle(context.Background(), m)
	if len(a.sent) != 0 {
		t.Fatalf("group message reached the agent: %v", a.sent)
	}
}

func TestRedeliveredMessageRunsOnce(t *testing.T) {
	s, a, _ := newSvc(t)
	a.script = func(a *fakeAgent) { a.ch <- core.Event{Type: core.EvTurnDone} }
	s.Handle(context.Background(), msg("same", "hello"))
	s.Handle(context.Background(), msg("same", "hello"))
	if len(a.sent) != 1 {
		t.Fatalf("agent received %d turns, want 1", len(a.sent))
	}
}

func TestApprovalIsForwardedAndDecidedByReply(t *testing.T) {
	s, a, rec := newSvc(t)
	released := make(chan struct{})
	a.script = func(a *fakeAgent) {
		a.ch <- core.Event{Type: core.EvApprovalRequest, Approval: &core.ApprovalRequest{ID: "ap1", Tool: "Bash", Title: "rm x"}}
		<-released
		a.ch <- core.Event{Type: core.EvTurnDone}
	}
	done := make(chan struct{})
	go func() { s.Handle(context.Background(), msg("m1", "do it")); close(done) }()
	waitFor(t, func() bool { return len(rec.all()) == 1 })
	s.Handle(context.Background(), msg("m2", "/ok"))
	close(released)
	<-done
	if len(a.decided) != 1 || !a.decided[0].Allow {
		t.Fatalf("decisions = %+v", a.decided)
	}
}

func TestBusyChatRejectsSecondMessage(t *testing.T) {
	s, a, rec := newSvc(t)
	release := make(chan struct{})
	a.script = func(a *fakeAgent) { <-release; a.ch <- core.Event{Type: core.EvTurnDone} }
	done := make(chan struct{})
	go func() { s.Handle(context.Background(), msg("m1", "first")); close(done) }()
	waitFor(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return len(a.sent) == 1 })
	s.Handle(context.Background(), msg("m2", "second"))
	close(release)
	<-done
	if len(a.sent) != 1 {
		t.Fatalf("second message was forwarded while busy: %v", a.sent)
	}
	if !strings.Contains(strings.Join(rec.all(), "|"), "/stop") {
		t.Fatalf("no busy notice: %q", rec.all())
	}
}

func TestWebhookRejectsWrongSecretAndAcceptsRightOne(t *testing.T) {
	s, a, _ := newSvc(t)
	a.script = func(a *fakeAgent) { a.ch <- core.Event{Type: core.EvTurnDone} }
	h := s.Handler()
	body := `{"ok":true,"result":{"event_name":"message.text.received","message":{"message_id":"w1","text":"hi","from":{"id":"u1","display_name":"A","is_bot":false},"chat":{"id":"c1","chat_type":"PRIVATE"}}}}`

	bad := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	req.Header.Set("X-Bot-Api-Secret-Token", "wrong")
	h.ServeHTTP(bad, req)
	if bad.Code != http.StatusForbidden {
		t.Fatalf("wrong secret: status %d, want 403", bad.Code)
	}

	good := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	req.Header.Set("X-Bot-Api-Secret-Token", "supersecret")
	h.ServeHTTP(good, req)
	if good.Code != http.StatusOK {
		t.Fatalf("right secret: status %d, want 200", good.Code)
	}
	waitFor(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return len(a.sent) == 1 })

	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, WebhookPath, nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: status %d, want 405", get.Code)
	}
}

func TestSplitTextKeepsPiecesUnderLimit(t *testing.T) {
	long := strings.Repeat("xin chào thế giới\n", 400)
	parts := splitText(long, maxText)
	if len(parts) < 2 {
		t.Fatalf("expected several parts, got %d", len(parts))
	}
	for _, p := range parts {
		if n := len([]rune(p)); n > maxText || n == 0 {
			t.Fatalf("part has %d runes", n)
		}
	}
}

func TestClientErrorNeverContainsToken(t *testing.T) {
	c := NewClient(Config{Token: "SECRET-TOKEN", APIBase: "http://127.0.0.1:1"})
	err := c.SendText(context.Background(), "c1", "hi")
	if err == nil || strings.Contains(err.Error(), "SECRET-TOKEN") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadConfigRequiresAllowListAndSecret(t *testing.T) {
	t.Setenv("ZALO_BOT_TOKEN", "t")
	t.Setenv("ZALO_WEBHOOK_SECRET", "short")
	t.Setenv("ZALO_ALLOWED_IDS", "a")
	if _, ok := LoadConfig(); ok {
		t.Fatal("a secret shorter than 8 characters must disable the integration")
	}
	t.Setenv("ZALO_WEBHOOK_SECRET", "long-enough-secret")
	t.Setenv("ZALO_ALLOWED_IDS", "")
	if _, ok := LoadConfig(); ok {
		t.Fatal("an empty allow-list must disable the integration")
	}
	t.Setenv("ZALO_ALLOWED_IDS", "a, b")
	cfg, ok := LoadConfig()
	if !ok || !cfg.Allowed["a"] || !cfg.Allowed["b"] || cfg.Mode != "plan" || cfg.Engine != "claude" {
		t.Fatalf("cfg = %+v ok=%v", cfg, ok)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

func TestWebhookAcceptsUnwrappedPayloadZaloReallySends(t *testing.T) {
	s, a, _ := newSvc(t)
	a.script = func(a *fakeAgent) { a.ch <- core.Event{Type: core.EvTurnDone} }
	body := `{"event_name":"message.text.received","message":{"date":1,"chat":{"chat_type":"PRIVATE","id":"c1"},"message_id":"real1","from":{"id":"u1","is_bot":false,"display_name":"A"},"text":"hi"}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	req.Header.Set("X-Bot-Api-Secret-Token", "supersecret")
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	waitFor(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return len(a.sent) == 1 && a.sent[0] == "hi" })
}

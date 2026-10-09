package zalo

import (
	"context"
	"errors"
	"fmt"
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
	switched []string
	convSets []store.ConvSettings
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
	a.convSets = append(a.convSets, c)
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
func (a *fakeAgent) SwitchEngine(conv, to string) error {
	a.mu.Lock()
	a.switched = append(a.switched, to)
	a.mu.Unlock()
	return nil
}
func (a *fakeAgent) State(string, string) core.State { return "" }

type memSettings struct {
	mu    sync.Mutex
	m     map[string]string
	convs map[string]*store.ConvSettings
}

func (s *memSettings) GetConv(conv string) (*store.ConvSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.convs[conv], nil
}

func (s *memSettings) GetSetting(k string) string { s.mu.Lock(); defer s.mu.Unlock(); return s.m[k] }
func (s *memSettings) SetSetting(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}

type fakeImages struct{}

func (fakeImages) Save(context.Context, string) (string, error) { return "/up/default.jpg", nil }

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
func (r *recorder) Typing(context.Context, string) error { return nil }
func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent...)
}

func newSvc(t *testing.T) (*Service, *fakeAgent, *recorder) {
	t.Helper()
	cfg := Config{Token: "tok", Secret: "supersecret", Allowed: map[string]bool{"u1": true}, Engine: "claude", Mode: "plan",
		Modes: []string{"plan", "ask"}, Workspace: "/ws"}
	a := &fakeAgent{ch: make(chan core.Event, 16)}
	rec := &recorder{}
	st := &memSettings{m: map[string]string{}, convs: map[string]*store.ConvSettings{}}
	n := 0
	s := New(cfg, Deps{
		Agent: a, Settings: st, Messenger: rec, Images: fakeImages{},
		NewConv: func(string, string) (string, error) { n++; return fmt.Sprintf("conv-%d", n), nil },
		PathOK:  func(p string) bool { return !strings.HasPrefix(p, "/secret") },
		Convs: func() ([]ConvInfo, error) {
			now := time.Now()
			return []ConvInfo{
				{ID: "old", Title: "cũ", Workspace: "/ws", Updated: now.Add(-48 * time.Hour)},
				{ID: "web-1", Title: "sửa lỗi build", Workspace: "/ws", Updated: now.Add(-time.Hour)},
				{ID: "hidden", Title: "ẩn", Workspace: "/ws", Updated: now, Archived: true},
				{ID: "forbidden", Title: "cấm", Workspace: "/secret/x", Updated: now},
			}, nil
		},
		Engines: func() []EngineInfo {
			return []EngineInfo{{ID: "claude", Modes: []string{"auto", "ask", "plan", "bypass"}}, {ID: "agy", Modes: []string{"plan", "bypass"}}}
		},
	})
	st.convs["web-1"] = &store.ConvSettings{ConvID: "web-1", ActiveEngine: "claude", Mode: "bypass", Workspace: "/ws"}
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

func say(s *Service, id, text string) { s.Handle(context.Background(), msg(id, text)) }

func lastReply(r *recorder) string {
	all := r.all()
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
}

func TestModeCommandRefusesRightsZaloMayNotGrant(t *testing.T) {
	s, a, rec := newSvc(t)
	say(s, "m1", "/mode bypass")
	if !strings.Contains(lastReply(rec), "không được đổi từ Zalo") {
		t.Fatalf("reply = %q", lastReply(rec))
	}
	for _, c := range a.convSets {
		if c.Mode == "bypass" {
			t.Fatalf("bypass was applied: %+v", c)
		}
	}
	say(s, "m2", "/mode ask")
	if got := a.convSets[len(a.convSets)-1]; got.Mode != "ask" {
		t.Fatalf("last SetConv = %+v, want mode ask", got)
	}
}

func TestEngineCommandSwitchesAndRejectsUnknownOrUnsupportedMode(t *testing.T) {
	s, a, rec := newSvc(t)
	say(s, "m1", "/engine nope")
	if !strings.Contains(lastReply(rec), "Không có engine") {
		t.Fatalf("reply = %q", lastReply(rec))
	}
	say(s, "m2", "/engine agy")
	if len(a.switched) != 1 || a.switched[0] != "agy" {
		t.Fatalf("switched = %v", a.switched)
	}
	if got := a.convSets[len(a.convSets)-1]; got.ActiveEngine != "agy" || got.Mode != "plan" {
		t.Fatalf("last SetConv = %+v", got)
	}
}

func TestWorkspaceCommandOpensNewConversationAndChecksPath(t *testing.T) {
	s, a, rec := newSvc(t)
	say(s, "m1", "/ws relative/dir")
	if !strings.Contains(lastReply(rec), "tuyệt đối") {
		t.Fatalf("reply = %q", lastReply(rec))
	}
	say(s, "m2", "/ws /secret/keys")
	if strings.Contains(lastReply(rec), "Đã mở") {
		t.Fatalf("forbidden folder accepted: %q", lastReply(rec))
	}
	dir := t.TempDir()
	say(s, "m3", "/ws "+dir)
	if !strings.Contains(lastReply(rec), "Đã mở hội thoại mới") {
		t.Fatalf("reply = %q", lastReply(rec))
	}
	if got := a.convSets[len(a.convSets)-1]; got.Workspace != dir {
		t.Fatalf("workspace = %q, want %q", got.Workspace, dir)
	}
}

func TestConvsListsOnlyAllowedRecentAndUseDowngradesRights(t *testing.T) {
	s, a, rec := newSvc(t)
	say(s, "m1", "/convs")
	list := lastReply(rec)
	if !strings.Contains(list, "sửa lỗi build") || strings.Contains(list, "ẩn") || strings.Contains(list, "cấm") {
		t.Fatalf("list = %q", list)
	}
	if strings.Index(list, "sửa lỗi build") > strings.Index(list, "cũ") {
		t.Fatalf("not newest first: %q", list)
	}
	say(s, "m2", "/use 1") // "sửa lỗi build" runs with bypass on the web
	if got := a.convSets[len(a.convSets)-1]; got.ConvID != "web-1" || got.Mode != "plan" {
		t.Fatalf("rights were not lowered: %+v", got)
	}
	if !strings.Contains(lastReply(rec), "hạ xuống plan") {
		t.Fatalf("reply = %q", lastReply(rec))
	}
	say(s, "m3", "/use 9")
	if !strings.Contains(lastReply(rec), "/convs") {
		t.Fatalf("reply = %q", lastReply(rec))
	}
}

func TestUnknownCommandIsNotSentToTheAgent(t *testing.T) {
	s, a, rec := newSvc(t)
	say(s, "m1", "/rm -rf /")
	if len(a.sent) != 0 || !strings.Contains(lastReply(rec), "/help") {
		t.Fatalf("sent=%v reply=%q", a.sent, lastReply(rec))
	}
}

func TestWatcherAnnouncesWebTurnsButNotZaloTurns(t *testing.T) {
	s, a, rec := newSvc(t)
	say(s, "m00", "/convs")
	say(s, "m0", "/use 1") // attaches chat c1 to web-1 and starts the watcher
	a.script = func(a *fakeAgent) {}
	a.ch <- core.Event{Type: core.EvUserMessage}
	a.ch <- core.Event{Type: core.EvTextDelta, Text: "đã sửa xong"}
	a.ch <- core.Event{Type: core.EvTurnDone}
	waitFor(t, func() bool { return strings.Contains(strings.Join(rec.all(), "|"), "đã sửa xong") })
	if !strings.Contains(lastReply(rec), "đã xong việc") {
		t.Fatalf("reply = %q", lastReply(rec))
	}
}

func TestProgressWaitsThenReportsOnlyNewTools(t *testing.T) {
	p := newProgress()
	p.add("Read")
	if p.due(p.start.Add(5*time.Second)) != "" {
		t.Fatal("reported too early")
	}
	if got := p.due(p.start.Add(25 * time.Second)); !strings.Contains(got, "Read") {
		t.Fatalf("got %q", got)
	}
	if p.due(p.start.Add(40*time.Second)) != "" {
		t.Fatal("repeated without anything new")
	}
	p.add("Bash")
	if p.due(p.start.Add(50*time.Second)) != "" {
		t.Fatal("reported again within 30s")
	}
	if got := p.due(p.start.Add(60 * time.Second)); !strings.Contains(got, "Bash") {
		t.Fatalf("got %q", got)
	}
}

func TestImageIsDownloadedAndHandedToTheAgent(t *testing.T) {
	s, a, _ := newSvc(t)
	s.d.Images = imageFunc(func(_ context.Context, url string) (string, error) {
		if url != "https://img.example/a.jpg" {
			t.Fatalf("url = %q", url)
		}
		return "/up/zalo-1.jpg", nil
	})
	a.script = func(a *fakeAgent) { a.ch <- core.Event{Type: core.EvTurnDone} }
	m := msg("img1", "đây là gì")
	m.Photo = "https://img.example/a.jpg"
	s.Handle(context.Background(), m)
	if len(a.sent) != 1 || !strings.Contains(a.sent[0], "đây là gì") || !strings.Contains(a.sent[0], "/up/zalo-1.jpg") {
		t.Fatalf("agent got %q", a.sent)
	}
}

func TestDownloadRefusesPrivateAddressesAndNonHTTPS(t *testing.T) {
	dir := t.TempDir()
	for _, u := range []string{"http://example.com/a.jpg", "https://127.0.0.1/a.jpg", "https://localhost/a.jpg", "https://192.168.1.10/a.jpg", "https://[::1]/a.jpg", "ftp://x/a"} {
		if p, err := NewImageStore(dir).Save(context.Background(), u); err == nil {
			t.Fatalf("%s accepted, saved %s", u, p)
		}
	}
}

func TestPhotoURLAcceptsStringObjectAndList(t *testing.T) {
	for in, want := range map[string]string{
		`"https://a/x.jpg"`:                                     "https://a/x.jpg",
		`{"url":"https://a/y.jpg"}`:                             "https://a/y.jpg",
		`["https://a/1.jpg","https://a/2.jpg"]`:                 "https://a/2.jpg",
		`[{"url":"https://a/1.jpg"},{"url":"https://a/3.jpg"}]`: "https://a/3.jpg",
		`null`: "", `42`: "",
	} {
		if got := photoURL([]byte(in)); got != want {
			t.Errorf("photoURL(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestWebhookPassesImageCaptionAndPhoto(t *testing.T) {
	s, a, _ := newSvc(t)
	got := make(chan Message, 1)
	s.d.Images = imageFunc(func(_ context.Context, url string) (string, error) {
		got <- Message{Photo: url}
		return "/up/x.jpg", nil
	})
	a.script = func(a *fakeAgent) { a.ch <- core.Event{Type: core.EvTurnDone} }
	body := `{"event_name":"message.image.received","message":{"message_id":"i1","photo":"https://img.example/p.png","caption":"nhìn nè","from":{"id":"u1","is_bot":false,"display_name":"A"},"chat":{"id":"c1","chat_type":"PRIVATE"}}}`
	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	req.Header.Set("X-Bot-Api-Secret-Token", "supersecret")
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)
	select {
	case m := <-got:
		if m.Photo != "https://img.example/p.png" {
			t.Fatalf("photo = %q", m.Photo)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("image never reached the service")
	}
}

type imageFunc func(ctx context.Context, url string) (string, error)

func (f imageFunc) Save(ctx context.Context, url string) (string, error) { return f(ctx, url) }

type fakeHost struct {
	mu      sync.Mutex
	actions []string // "name action"
	err     error
}

func (h *fakeHost) Overview(context.Context) (string, error) {
	return "🖥 duinch — chạy 3 giờ", nil
}
func (h *fakeHost) Services(context.Context) ([]HostService, error) {
	return []HostService{{"sonarr", "🟢 PVR for usenet"}, {"cloudflared-dashboard", "🟢 tunnel"}}, nil
}
func (h *fakeHost) ServiceStatus(_ context.Context, name string) (string, error) {
	if name == "nope" {
		return "", errors.New("không có service này (xem /nas service-list)")
	}
	return "🟢 " + name + " — running", nil
}
func (h *fakeHost) ServiceAction(_ context.Context, name, action string) (string, error) {
	h.mu.Lock()
	h.actions = append(h.actions, name+" "+action)
	h.mu.Unlock()
	return "✅ " + name + " " + action, h.err
}

func nasSvc(t *testing.T, control bool) (*Service, *fakeHost, *recorder) {
	t.Helper()
	s, _, rec := newSvc(t)
	h := &fakeHost{}
	s.d.Host = h
	s.cfg.NASControl = control
	s.cfg.NASProtected = []string{"cloudflared", "agent-bridge"}
	return s, h, rec
}

func TestNASReportsMachineListAndHelpWithoutAnAgent(t *testing.T) {
	s, _, rec := nasSvc(t, false)
	say(s, "n1", "/nas")
	if !strings.Contains(lastReply(rec), "chạy 3 giờ") {
		t.Fatalf("overview = %q", lastReply(rec))
	}
	say(s, "n2", "/nas service-list")
	if got := lastReply(rec); !strings.Contains(got, "sonarr — 🟢 PVR for usenet") || !strings.Contains(got, "2 service") {
		t.Fatalf("list = %q", got)
	}
	say(s, "n3", "/nas help")
	if !strings.Contains(lastReply(rec), "/nas:<service> start|stop|restart|update") {
		t.Fatalf("help = %q", lastReply(rec))
	}
	say(s, "n4", "/nas:sonarr")
	if lastReply(rec) != "🟢 sonarr — running" {
		t.Fatalf("status = %q", lastReply(rec))
	}
	say(s, "n5", "/nas:nope")
	if !strings.HasPrefix(lastReply(rec), "❌ không có service này") {
		t.Fatalf("unknown = %q", lastReply(rec))
	}
	say(s, "n6", "/nas banana")
	if !strings.Contains(lastReply(rec), "/nas help") {
		t.Fatalf("unknown sub = %q", lastReply(rec))
	}
}

func TestNASControlIsOffByDefaultAndWhenOnProtectsTheBotsOwnServices(t *testing.T) {
	s, h, rec := nasSvc(t, false)
	say(s, "a1", "/nas:sonarr restart")
	if !strings.Contains(lastReply(rec), "ZALO_NAS_CONTROL=1") || len(h.actions) != 0 {
		t.Fatalf("control off: reply=%q actions=%v", lastReply(rec), h.actions)
	}
	s, h, rec = nasSvc(t, true)
	say(s, "a2", "/nas:sonarr restart")
	say(s, "a3", "/nas:SONARR update")
	say(s, "a4", "/nas:cloudflared-dashboard stop")
	say(s, "a5", "/nas:agent-bridge restart")
	say(s, "a6", "/nas:cloudflared-dashboard start") // starting is harmless
	say(s, "a7", "/nas:sonarr explode")
	want := []string{"sonarr restart", "SONARR update", "cloudflared-dashboard start"}
	if strings.Join(h.actions, "|") != strings.Join(want, "|") {
		t.Fatalf("actions = %v, want %v", h.actions, want)
	}
	if !strings.Contains(strings.Join(rec.all(), "|"), "dùng SSH") || !strings.Contains(lastReply(rec), "không có") {
		t.Fatalf("replies = %q", rec.all())
	}
}

func TestNASWorksWhileTheAgentIsBusyAndIsOffWithoutAHost(t *testing.T) {
	s, _, rec := nasSvc(t, false)
	c := s.chat("c1")
	c.busy.Lock() // an agent turn is running
	say(s, "b1", "/nas")
	c.busy.Unlock()
	if !strings.Contains(lastReply(rec), "chạy 3 giờ") {
		t.Fatalf("reply = %q", lastReply(rec))
	}
	s.d.Host = nil
	say(s, "b2", "/nas")
	if !strings.Contains(lastReply(rec), "chưa được bật") {
		t.Fatalf("reply = %q", lastReply(rec))
	}
}

func TestQualifierIsOnlyAcceptedByCommandsThatTakeOne(t *testing.T) {
	s, a, rec := newSvc(t)
	say(s, "q1", "/mode:bypass")
	if !strings.Contains(lastReply(rec), "/help") || len(a.convSets) != 0 {
		t.Fatalf("reply=%q sets=%v", lastReply(rec), a.convSets)
	}
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nchungdev/agent-hub/internal/core"
	"github.com/nchungdev/agent-hub/internal/manager"
	"github.com/nchungdev/agent-hub/internal/session"
	"github.com/nchungdev/agent-hub/internal/store"
)

// V2 is the engine-agnostic transport: one WebSocket protocol for every CLI engine.
type V2 struct {
	cmdMu      sync.Mutex
	cmdCache   map[string]cachedCommands
	modelMu    sync.Mutex
	modelCache map[string]cachedModels
	// DataDir holds the persisted model cache (models-cache.json).
	DataDir     string
	loginMu     sync.Mutex
	logins      map[string]*loginFlow
	statusMu    sync.Mutex
	statusCache map[string]cachedAuth
	Mgr         *manager.Manager
	Store       *store.Store
	Engines     []core.Engine
	Convs       *session.Manager // existing conversation list (sessions table)
	// DefaultWorkspace is used when a conversation has none (must pass PathAllowed).
	DefaultWorkspace string
}

var v2Upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// Same-origin only: a foreign web page must not be able to drive agents.
	CheckOrigin: sameOrigin,
}

func (v *V2) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/engines", v.handleEngines)
	mux.HandleFunc("POST /api/v2/models/refresh", v.handleRefreshModels)
	mux.HandleFunc("GET /api/v2/engines/{id}/commands", v.handleCommands)
	mux.HandleFunc("GET /api/v2/conv/{id}", v.handleConv)
	mux.HandleFunc("GET /api/v2/conv/{id}/events", v.handleEvents)
	mux.HandleFunc("POST /api/v2/engines/{id}/login", v.handleLoginStart)
	mux.HandleFunc("GET /api/v2/engines/{id}/login", v.handleLoginState)
	mux.HandleFunc("POST /api/v2/engines/{id}/login/input", v.handleLoginInput)
	mux.HandleFunc("DELETE /api/v2/engines/{id}/login", v.handleLoginCancel)
	mux.HandleFunc("/ws/v2", v.handleWS)
}

type engineInfo struct {
	Auth            *core.AuthStatus  `json:"auth,omitempty"`
	ID              string            `json:"id"`
	Capabilities    core.Capabilities `json:"capabilities"`
	CanLogin        bool              `json:"can_login"`
	ModelsFetchedAt time.Time         `json:"models_fetched_at"`
	Models          []core.Model      `json:"models"`
}

func (v *V2) handleEngines(w http.ResponseWriter, r *http.Request) {
	out := []engineInfo{}
	for _, e := range v.Engines {
		cm := v.cachedModels(r.Context(), e, false)
		ms := cm.Models
		_, canLogin := e.(core.LoginProvider)
		info := engineInfo{ID: e.ID(), Capabilities: e.Capabilities(), Models: ms, CanLogin: canLogin, ModelsFetchedAt: cm.FetchedAt}
		if sp, ok := e.(core.StatusProvider); ok {
			st := v.cachedStatus(r.Context(), e.ID(), sp, r.URL.Query().Get("refresh") == "1")
			info.Auth = &st
		}
		out = append(out, info)
	}
	jsonResponse(w, out)
}

func (v *V2) convSnapshot(id string) map[string]any {
	c, _ := v.Store.GetConv(id)
	b, _ := v.Store.ListBindings(id)
	p, _ := v.Store.PendingApprovals(id)
	ws, _ := v.Store.GetState(id)
	live := map[string]string{}
	for _, e := range v.Engines {
		if st := v.Mgr.State(id, e.ID()); st != "" {
			live[e.ID()] = string(st)
		}
	}
	return map[string]any{"conv": c, "bindings": b, "pending_approvals": p, "working_state": ws, "live": live}
}

func (v *V2) handleConv(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, v.convSnapshot(r.PathValue("id")))
}

func (v *V2) handleEvents(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	jsonResponse(w, v.history(r.PathValue("id"), since))
}

type v2Req struct {
	Type      string `json:"type"`
	Conv      string `json:"conv"`
	Engine    string `json:"engine"`
	Text      string `json:"text"`
	Model     string `json:"model"`
	Mode      string `json:"mode"`
	Workspace string `json:"workspace"`
	Approval  string `json:"approval_id"`
	Allow     bool   `json:"allow"`
	Scope     string `json:"scope"`
	Since     int64  `json:"since"`
	Effort    string `json:"effort"`
	Confirmed bool   `json:"confirmed"`
	Media     []struct {
		MimeType string `json:"mime_type"`
		URI      string `json:"uri"`
	} `json:"media"`
}

type v2Conn struct {
	ws   *websocket.Conn
	mu   sync.Mutex
	subs map[string]func()
}

func (c *v2Conn) write(v any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = c.ws.WriteJSON(v)
}

func (v *V2) handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := v2Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws-v2] upgrade: %v", err)
		return
	}
	c := &v2Conn{ws: ws, subs: map[string]func(){}}
	defer func() {
		for _, cancel := range c.subs {
			cancel()
		}
		ws.Close()
	}()
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		var req v2Req
		if err := json.Unmarshal(data, &req); err != nil {
			c.write(map[string]any{"type": "error", "message": "invalid JSON"})
			continue
		}
		v.dispatch(c, req)
	}
}

func (v *V2) subscribe(c *v2Conn, conv string) {
	if _, ok := c.subs[conv]; ok {
		return
	}
	ch, cancel := v.Mgr.Subscribe(conv)
	c.subs[conv] = cancel
	go func() {
		for ev := range ch {
			c.write(map[string]any{"type": "event", "event": ev})
		}
	}()
}

func (v *V2) dispatch(c *v2Conn, req v2Req) {
	fail := func(err error) { c.write(map[string]any{"type": "error", "conv": req.Conv, "message": err.Error()}) }
	switch req.Type {
	case "subscribe":
		v.subscribe(c, req.Conv)
		evs := v.history(req.Conv, req.Since)
		c.write(map[string]any{"type": "snapshot", "conv": req.Conv, "snapshot": v.convSnapshot(req.Conv), "events": evs})
	case "shell":
		cmd := strings.TrimSpace(req.Text)
		if cmd == "" {
			fail(fmt.Errorf("empty command"))
			return
		}
		if manager.IsPrivileged(cmd) && !req.Confirmed {
			fail(errNeedsConfirm)
			return
		}
		conv := req.Conv
		if conv == "" {
			s, err := v.Convs.CreateSession("New Chat", req.Workspace)
			if err != nil {
				fail(err)
				return
			}
			conv = s.ID
			c.write(map[string]any{"type": "conv_created", "conv": conv})
		}
		v.subscribe(c, conv)
		v.importLegacy(conv)
		cs, _ := v.Store.GetConv(conv)
		ws := req.Workspace
		if cs != nil && cs.Workspace != "" {
			ws = cs.Workspace
		}
		if ws == "" {
			ws = v.DefaultWorkspace
		}
		if !PathAllowed(ws) {
			fail(errForbidden)
			return
		}
		if cs == nil {
			_ = v.Store.SetConv(store.ConvSettings{ConvID: conv, ActiveEngine: req.Engine, Mode: req.Mode, Workspace: ws, Model: req.Model, Effort: req.Effort})
		}
		v.nameConv(conv, "!"+cmd)
		go v.runShell(conv, cmd, ws)
	case "send":
		conv := req.Conv
		if conv == "" {
			s, err := v.Convs.CreateSession("New Chat", req.Workspace)
			if err != nil {
				fail(err)
				return
			}
			conv = s.ID
			c.write(map[string]any{"type": "conv_created", "conv": conv})
		}
		v.subscribe(c, conv)
		cs, _ := v.Store.GetConv(conv)
		v.importLegacy(conv)
		set := store.ConvSettings{ConvID: conv, ActiveEngine: req.Engine, Mode: req.Mode, Workspace: req.Workspace, Model: req.Model, Effort: req.Effort}
		if cs != nil {
			if set.Mode == "" {
				set.Mode = cs.Mode
			}
			if set.Workspace == "" {
				set.Workspace = cs.Workspace
			}
			if set.ActiveEngine == "" {
				set.ActiveEngine = cs.ActiveEngine
			}
		}
		if set.ActiveEngine == "" {
			fail(core.ErrNoSuchEngine)
			return
		}
		if set.Workspace == "" {
			set.Workspace = v.DefaultWorkspace
		}
		if set.Workspace != "" && !PathAllowed(set.Workspace) {
			fail(errForbidden)
			return
		}
		if err := v.checkMode(set.ActiveEngine, set.Mode); err != nil {
			fail(err)
			return
		}
		prev := ""
		if cs != nil {
			prev = cs.ActiveEngine
		}
		if prev != "" && prev != set.ActiveEngine {
			if err := v.Mgr.SwitchEngine(conv, set.ActiveEngine); err != nil {
				fail(err)
				return
			}
		}
		if err := v.Mgr.SetConv(set); err != nil {
			fail(err)
			return
		}
		text := req.Text
		if len(req.Media) > 0 {
			var sb strings.Builder
			sb.WriteString(text + "\n\nAttached media:")
			for _, m := range req.Media {
				sb.WriteString("\n- " + m.URI)
			}
			text = sb.String()
		}
		v.nameConv(conv, req.Text)
		if err := v.Mgr.Send(context.Background(), conv, set.ActiveEngine, core.UserInput{Text: text}); err != nil {
			fail(err)
		}
	case "decide":
		scope := req.Scope
		if scope == "" {
			scope = "once"
		}
		if err := v.Mgr.Decide(req.Conv, req.Approval, core.Decision{Allow: req.Allow, Scope: scope}); err != nil {
			fail(err)
		}
	case "cancel":
		_ = v.Mgr.Cancel(req.Conv)
	case "set_mode":
		cs, _ := v.Store.GetConv(req.Conv)
		if cs != nil {
			if err := v.checkMode(cs.ActiveEngine, req.Mode); err != nil {
				fail(err)
				return
			}
		}
		if err := v.Mgr.SetConv(store.ConvSettings{ConvID: req.Conv, Mode: req.Mode}); err != nil {
			fail(err)
		}
	case "switch_engine":
		if err := v.Mgr.SwitchEngine(req.Conv, req.Engine); err != nil {
			fail(err)
		}
	default:
		fail(http.ErrNotSupported)
	}
}

type cachedAuth struct {
	st core.AuthStatus
	at time.Time
}

func (v *V2) cachedStatus(ctx context.Context, id string, sp core.StatusProvider, force bool) core.AuthStatus {
	v.statusMu.Lock()
	if v.statusCache == nil {
		v.statusCache = map[string]cachedAuth{}
	}
	if c, ok := v.statusCache[id]; ok && !force && time.Since(c.at) < 10*time.Second {
		v.statusMu.Unlock()
		return c.st
	}
	v.statusMu.Unlock()
	st := sp.Status(ctx)
	v.statusMu.Lock()
	v.statusCache[id] = cachedAuth{st: st, at: time.Now()}
	v.statusMu.Unlock()
	return st
}

// checkMode rejects permission modes the engine cannot enforce.
func (v *V2) checkMode(engine, mode string) error {
	if mode == "" {
		return nil
	}
	for _, e := range v.Engines {
		if e.ID() != engine {
			continue
		}
		modes := e.Capabilities().PermissionModes
		if len(modes) == 0 {
			return nil
		}
		for _, m := range modes {
			if m == mode {
				return nil
			}
		}
		return fmt.Errorf("%s does not support permission mode %q (supported: %s)", engine, mode, strings.Join(modes, ", "))
	}
	return core.ErrNoSuchEngine
}

// nameConv titles a new conversation after its first prompt and bumps it in the list.
func (v *V2) nameConv(conv, text string) {
	if v.Convs == nil {
		return
	}
	if sess, err := v.Convs.GetSession(conv); err == nil && (sess.Name == "" || sess.Name == "New Chat") {
		title := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
		if r := []rune(title); len(r) > 60 {
			title = string(r[:60]) + "…"
		}
		if title != "" {
			_ = v.Convs.RenameSession(conv, title)
			return
		}
	}
	v.Convs.TouchSession(conv)
}

// legacyEvents renders chats created by the pre-v2 hub (messages table) as read-only
// events with negative sequence numbers, so they open like any other conversation.
func (v *V2) legacyEvents(conv string) []core.Event {
	if v.Convs == nil {
		return nil
	}
	msgs, err := v.Convs.GetMessages(conv, 500)
	if err != nil || len(msgs) == 0 {
		return nil
	}
	var out []core.Event
	add := func(ev core.Event) { ev.ConvID = conv; out = append(out, ev) }
	for _, m := range msgs {
		if m.Role == "user" {
			add(core.Event{Type: core.EvUserMessage, Text: m.Content, Time: m.CreatedAt})
			continue
		}
		for i, st := range m.Steps {
			id := fmt.Sprintf("legacy-%s-%d", m.ID, i)
			args := map[string]any{"command": st.Command, "path": st.Path}
			add(core.Event{Type: core.EvToolCall, Engine: m.Agent, Tool: &core.ToolCall{ID: id, Name: st.Name, Args: args}, Time: m.CreatedAt})
			add(core.Event{Type: core.EvToolResult, Engine: m.Agent, Tool: &core.ToolCall{ID: id, Output: st.Output}, Time: m.CreatedAt})
		}
		add(core.Event{Type: core.EvTextDelta, Engine: m.Agent, Model: m.Model, Text: m.Content, Time: m.CreatedAt})
		add(core.Event{Type: core.EvTurnDone, Engine: m.Agent, Time: m.CreatedAt})
	}
	n := int64(len(out))
	for i := range out {
		out[i].Seq = int64(i) - n // -n .. -1
	}
	return out
}

// history returns stored events after `since`, falling back to legacy messages for old chats.
func (v *V2) history(conv string, since int64) []core.Event {
	evs, _ := v.Store.Since(conv, since, 2000)
	if len(evs) == 0 && since == 0 {
		if first, _ := v.Store.Tail(conv, 1); len(first) == 0 {
			evs = v.legacyEvents(conv)
		}
	}
	if evs == nil {
		evs = []core.Event{} // JSON [] not null: clients iterate it
	}
	return evs
}

// importLegacy copies an old chat into the event log the first time it is continued,
// so the new engine can receive it as context and sequence numbers stay consistent.
func (v *V2) importLegacy(conv string) {
	if first, _ := v.Store.Tail(conv, 1); len(first) > 0 {
		return
	}
	for _, ev := range v.legacyEvents(conv) {
		ev.Seq = 0
		if _, err := v.Store.Append(ev); err != nil {
			log.Printf("[ws-v2] import legacy: %v", err)
			return
		}
	}
}

type cachedModels struct {
	Models    []core.Model `json:"models"`
	FetchedAt time.Time    `json:"fetched_at"`
}

const modelTTL = 24 * time.Hour

func (v *V2) modelCachePath() string {
	if v.DataDir == "" {
		return ""
	}
	return filepath.Join(v.DataDir, "models-cache.json")
}

// loadModelCache reads the persisted cache once (so restarts do not refetch).
func (v *V2) loadModelCache() {
	if v.modelCache != nil {
		return
	}
	v.modelCache = map[string]cachedModels{}
	if p := v.modelCachePath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &v.modelCache)
		}
	}
}

func (v *V2) saveModelCacheLocked() {
	p := v.modelCachePath()
	if p == "" {
		return
	}
	if b, err := json.Marshal(v.modelCache); err == nil {
		tmp := p + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			_ = os.Rename(tmp, p)
		}
	}
}

// cachedModels returns the engine's model list, fetching only when missing
// (first open), older than 24h, or when force is set (the Refresh button).
func (v *V2) cachedModels(ctx context.Context, e core.Engine, force bool) cachedModels {
	v.modelMu.Lock()
	v.loadModelCache()
	if c, ok := v.modelCache[e.ID()]; ok && !force && time.Since(c.FetchedAt) < modelTTL {
		v.modelMu.Unlock()
		return c
	}
	v.modelMu.Unlock()
	ms, _ := e.Models(ctx)
	c := cachedModels{Models: ms, FetchedAt: time.Now().UTC()}
	// A listing engine that returned only its fallback entry probably failed: retry soon.
	if e.Capabilities().ModelListing && len(ms) <= 1 {
		c.FetchedAt = time.Now().UTC().Add(-modelTTL + 2*time.Minute)
	}
	v.modelMu.Lock()
	v.modelCache[e.ID()] = c
	v.saveModelCacheLocked()
	v.modelMu.Unlock()
	return c
}

func (v *V2) handleRefreshModels(w http.ResponseWriter, r *http.Request) {
	only := r.URL.Query().Get("engine")
	for _, e := range v.Engines {
		if only == "" || only == e.ID() {
			v.cachedModels(r.Context(), e, true)
		}
	}
	v.handleEngines(w, r)
}

// Warm fills the model/auth caches so the first page load is instant.
func (v *V2) Warm() {
	for _, e := range v.Engines {
		v.cachedModels(context.Background(), e, false)
		if sp, ok := e.(core.StatusProvider); ok {
			v.cachedStatus(context.Background(), e.ID(), sp, true)
		}
	}
}

type cachedCommands struct {
	cmds []core.Command
	at   time.Time
}

const commandTTL = 6 * time.Hour

// handleCommands returns the engine's slash commands / skills for the "/" menu.
func (v *V2) handleCommands(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var lister core.CommandLister
	for _, e := range v.Engines {
		if e.ID() == id {
			lister, _ = e.(core.CommandLister)
		}
	}
	if lister == nil {
		jsonResponse(w, []core.Command{})
		return
	}
	v.cmdMu.Lock()
	if v.cmdCache == nil {
		v.cmdCache = map[string]cachedCommands{}
	}
	c, ok := v.cmdCache[id]
	v.cmdMu.Unlock()
	if !ok || r.URL.Query().Get("refresh") == "1" || time.Since(c.at) > commandTTL {
		cmds, err := lister.Commands(r.Context())
		if err != nil {
			if ok { // serve stale rather than nothing
				jsonResponse(w, c.cmds)
				return
			}
			httpError(w, err, http.StatusBadGateway)
			return
		}
		if cmds == nil {
			cmds = []core.Command{}
		}
		c = cachedCommands{cmds: cmds, at: time.Now()}
		v.cmdMu.Lock()
		v.cmdCache[id] = c
		v.cmdMu.Unlock()
	}
	jsonResponse(w, c.cmds)
}

const errNeedsConfirm = constErr("this command escalates privileges; confirm it explicitly")

const (
	shellTimeout   = 120 * time.Second
	shellOutputCap = 64 * 1024
)

// capWriter keeps the first shellOutputCap bytes and counts the rest.
type capWriter struct {
	buf     bytes.Buffer
	dropped int
}

func (w *capWriter) Write(p []byte) (int, error) {
	room := shellOutputCap - w.buf.Len()
	if room > len(p) {
		room = len(p)
	}
	if room > 0 {
		w.buf.Write(p[:room])
	}
	w.dropped += len(p) - room
	return len(p), nil
}

// runShell executes a user-typed "!cmd" in the workspace (no model involved) and records the
// result as an EvShell event, which later turns receive as context.
func (v *V2) runShell(conv, command, workspace string) {
	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "TERM=dumb", "NO_COLOR=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	var out capWriter
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	timedOut := ctx.Err() == context.DeadlineExceeded
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 127
			out.buf.WriteString(err.Error())
		}
	}
	text := out.buf.String()
	if out.dropped > 0 {
		text += fmt.Sprintf("\n…[%d more bytes not shown]", out.dropped)
	}
	if timedOut {
		text += fmt.Sprintf("\n…[stopped after %s]", shellTimeout)
	}
	v.Mgr.Emit(core.Event{ConvID: conv, Engine: "shell", Type: core.EvShell,
		Tool: &core.ToolCall{ID: fmt.Sprintf("shell-%d", time.Now().UnixNano()), Name: "Bash", Args: map[string]any{"command": command, "cwd": workspace}, Output: text},
		Data: map[string]any{"exit_code": code, "timed_out": timedOut}})
}

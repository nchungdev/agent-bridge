package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nchungdev/agent-hub/internal/core"
	"github.com/nchungdev/agent-hub/internal/manager"
	"github.com/nchungdev/agent-hub/internal/session"
	"github.com/nchungdev/agent-hub/internal/store"
)

// V2 is the engine-agnostic transport: one WebSocket protocol for every CLI engine.
type V2 struct {
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
	mux.HandleFunc("GET /api/v2/conv/{id}", v.handleConv)
	mux.HandleFunc("GET /api/v2/conv/{id}/events", v.handleEvents)
	mux.HandleFunc("POST /api/v2/engines/{id}/login", v.handleLoginStart)
	mux.HandleFunc("GET /api/v2/engines/{id}/login", v.handleLoginState)
	mux.HandleFunc("POST /api/v2/engines/{id}/login/input", v.handleLoginInput)
	mux.HandleFunc("DELETE /api/v2/engines/{id}/login", v.handleLoginCancel)
	mux.HandleFunc("/ws/v2", v.handleWS)
}

type engineInfo struct {
	Auth         *core.AuthStatus  `json:"auth,omitempty"`
	ID           string            `json:"id"`
	Capabilities core.Capabilities `json:"capabilities"`
	CanLogin     bool              `json:"can_login"`
	Models       []core.Model      `json:"models"`
}

func (v *V2) handleEngines(w http.ResponseWriter, r *http.Request) {
	out := []engineInfo{}
	for _, e := range v.Engines {
		ms, _ := e.Models(r.Context())
		_, canLogin := e.(core.LoginProvider)
		info := engineInfo{ID: e.ID(), Capabilities: e.Capabilities(), Models: ms, CanLogin: canLogin}
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
	evs, err := v.Store.Since(r.PathValue("id"), since, 2000)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	jsonResponse(w, evs)
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
		evs, _ := v.Store.Since(req.Conv, req.Since, 2000)
		c.write(map[string]any{"type": "snapshot", "conv": req.Conv, "snapshot": v.convSnapshot(req.Conv), "events": evs})
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
		set := store.ConvSettings{ConvID: conv, ActiveEngine: req.Engine, Mode: req.Mode, Workspace: req.Workspace}
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
		if err := v.Mgr.Send(context.Background(), conv, set.ActiveEngine, core.UserInput{Text: req.Text}); err != nil {
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

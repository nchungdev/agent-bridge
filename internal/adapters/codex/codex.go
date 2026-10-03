// Package codex adapts `codex app-server` (JSON-RPC over stdio), which exposes
// real approval requests, to the engine-agnostic core.
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nchungdev/agent-hub/internal/adapters/proc"
	"github.com/nchungdev/agent-hub/internal/core"
)

type Engine struct{ Bin string }

func New(bin string) *Engine {
	if bin == "" {
		if p, err := exec.LookPath("codex"); err == nil {
			bin = p
		}
	}
	return &Engine{Bin: bin}
}

func (e *Engine) ID() string { return "codex" }
func (e *Engine) Capabilities() core.Capabilities {
	return core.Capabilities{Streaming: true, Resume: true, PermissionPrompts: true, PlanMode: true, ModelListing: true,
		PermissionModes: []string{"ask", "plan", "accept-edits", "bypass"}}
}

// Status uses `codex login status` ("Not logged in" / "Logged in using ...").
func (e *Engine) Status(ctx context.Context) core.AuthStatus {
	st := core.AuthStatus{LoginHint: "Use the Codex device login in the classic UI (Settings → Add AI engine), or run: codex login --device-auth"}
	if e.Bin == "" {
		st.Detail = "codex CLI not found"
		st.Known = true
		return st
	}
	st.Installed = true
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, e.Bin, "login", "status").CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil && text == "" {
		st.Detail = "could not query login status"
		return st
	}
	st.Known = true
	st.LoggedIn = err == nil && !strings.Contains(strings.ToLower(text), "not logged in")
	st.Detail = text
	return st
}

func (e *Engine) Models(ctx context.Context) ([]core.Model, error) {
	fallback := []core.Model{{ID: "", Name: "Codex default"}}
	if e.Bin == "" {
		return fallback, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	c, err := dial(cctx, e.Bin, "")
	if err != nil {
		return fallback, nil
	}
	defer c.p.Close()
	raw, err := c.call(cctx, "model/list", map[string]any{})
	if err != nil {
		return fallback, nil
	}
	var r struct {
		Data []struct {
			ID, Model, DisplayName string
			Hidden                 bool
		} `json:"data"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return fallback, nil
	}
	var out []core.Model
	for _, m := range r.Data {
		if m.Hidden {
			continue
		}
		id := m.Model
		if id == "" {
			id = m.ID
		}
		name := m.DisplayName
		if name == "" {
			name = id
		}
		out = append(out, core.Model{ID: id, Name: name})
	}
	if len(out) == 0 {
		return fallback, nil
	}
	return out, nil
}

// JSON-RPC client -----------------------------------------------------------

type rpcMsg struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type client struct {
	p       *proc.Proc
	mu      sync.Mutex
	nextID  int
	waiting map[string]chan rpcMsg
	onMsg   func(rpcMsg) // notifications and server requests
	done    chan struct{}
}

func dial(ctx context.Context, bin, dir string) (*client, error) {
	p, err := proc.Start(ctx, proc.Options{Bin: bin, Args: []string{"app-server"}, Dir: dir, Env: []string{"TERM=dumb", "NO_COLOR=1"}})
	if err != nil {
		return nil, err
	}
	c := &client{p: p, waiting: map[string]chan rpcMsg{}, done: make(chan struct{})}
	go c.readLoop()
	if _, err := c.call(ctx, "initialize", map[string]any{"clientInfo": map[string]any{"name": "agent-hub", "title": "Agent Hub", "version": "2"}}); err != nil {
		p.Close()
		return nil, fmt.Errorf("codex initialize: %w", err)
	}
	_ = c.notify("initialized", nil)
	return c, nil
}

func (c *client) readLoop() {
	defer close(c.done)
	for {
		line, err := c.p.Stdout.ReadBytes('\n')
		if len(line) > 0 {
			var m rpcMsg
			if json.Unmarshal(line, &m) == nil {
				c.route(m)
			}
		}
		if err != nil {
			if err != io.EOF {
				_ = err
			}
			<-c.p.Done()
			c.mu.Lock()
			for k, ch := range c.waiting {
				close(ch)
				delete(c.waiting, k)
			}
			c.mu.Unlock()
			return
		}
	}
}

func (c *client) route(m rpcMsg) {
	if m.Method == "" && len(m.ID) > 0 { // response
		c.mu.Lock()
		ch := c.waiting[string(m.ID)]
		delete(c.waiting, string(m.ID))
		c.mu.Unlock()
		if ch != nil {
			ch <- m
		}
		return
	}
	if c.onMsg != nil {
		c.onMsg(m)
	}
}

func (c *client) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.p.WriteLine(b)
}

func (c *client) notify(method string, params any) error {
	m := map[string]any{"method": method}
	if params != nil {
		m["params"] = params
	}
	return c.write(m)
}

func (c *client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := strconv.Itoa(c.nextID)
	ch := make(chan rpcMsg, 1)
	c.waiting[id] = ch
	c.mu.Unlock()
	if err := c.write(map[string]any{"id": c.nextID, "method": method, "params": params}); err != nil {
		return nil, err
	}
	select {
	case m, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("codex exited (%s)", c.p.StderrTail())
		}
		if m.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		return m.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *client) respond(id json.RawMessage, result any) error {
	return c.write(map[string]any{"id": json.RawMessage(id), "result": result})
}

func (c *client) respondErr(id json.RawMessage, code int, msg string) error {
	return c.write(map[string]any{"id": json.RawMessage(id), "error": map[string]any{"code": code, "message": msg}})
}

// Session --------------------------------------------------------------------

func sandboxFor(mode string) string {
	if mode == "plan" {
		return "read-only"
	}
	return "workspace-write"
}

func (e *Engine) Start(ctx context.Context, o core.StartOpts) (core.Session, error) {
	if e.Bin == "" {
		return nil, fmt.Errorf("codex binary not found in PATH")
	}
	// The process must outlive the request ctx; only the handshake is time-boxed.
	c, err := dial(context.Background(), e.Bin, o.Workspace)
	if err != nil {
		return nil, err
	}
	s := &session{c: c, events: make(chan core.Event, 256), pending: map[string]pendingReq{}, model: o.Model, effort: o.Effort, cwd: o.Workspace, mode: o.Mode}
	c.onMsg = s.onMsg
	hctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	params := map[string]any{"approvalPolicy": "on-request", "sandbox": sandboxFor(o.Mode)}
	if o.Workspace != "" {
		params["cwd"] = o.Workspace
	}
	if o.Model != "" {
		params["model"] = o.Model
	}
	method := "thread/start"
	if o.ResumeID != "" {
		method = "thread/resume"
		params["threadId"] = o.ResumeID
	}
	raw, err := c.call(hctx, method, params)
	if err != nil {
		c.p.Close()
		return nil, err
	}
	var r struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Thread.ID == "" {
		c.p.Close()
		return nil, fmt.Errorf("codex %s: no thread id in response", method)
	}
	s.threadID = r.Thread.ID
	go func() { // close the event stream when the process ends
		<-c.done
		s.closeEvents()
	}()
	return s, nil
}

type pendingReq struct {
	rpcID  json.RawMessage
	kind   string // command | file | permissions
	params json.RawMessage
}

type session struct {
	c        *client
	threadID string
	events   chan core.Event
	model    string
	effort   string
	cwd      string

	mu       sync.Mutex
	closed   bool
	mode     string
	turnID   string
	pending  map[string]pendingReq
	lastTok  *core.Usage
	failedFn bool
}

func (s *session) EngineSessionID() string   { return s.threadID }
func (s *session) Events() <-chan core.Event { return s.events }

func (s *session) closeEvents() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.events)
	}
}

func (s *session) emit(ev core.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	ev.Time = time.Now()
	s.events <- ev
}

func (s *session) Send(ctx context.Context, in core.UserInput) error {
	text := in.Text
	if in.Preamble != "" {
		text = "<context>\n" + in.Preamble + "\n</context>\n\n" + text
	}
	s.mu.Lock()
	s.lastTok = nil
	mode := s.mode
	s.mu.Unlock()
	params := map[string]any{
		"threadId":       s.threadID,
		"input":          []any{map[string]any{"type": "text", "text": text, "text_elements": []any{}}},
		"approvalPolicy": "on-request",
	}
	if mode == "plan" {
		params["sandboxPolicy"] = map[string]any{"type": "readOnly"}
	}
	if s.cwd != "" {
		params["cwd"] = s.cwd
	}
	if s.model != "" {
		params["model"] = s.model
	}
	if s.effort != "" {
		params["effort"] = strings.ToLower(s.effort)
	}
	raw, err := s.c.call(ctx, "turn/start", params)
	if err != nil {
		return err
	}
	var r struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if json.Unmarshal(raw, &r) == nil && r.Turn.ID != "" {
		s.mu.Lock()
		s.turnID = r.Turn.ID
		s.mu.Unlock()
	}
	return nil
}

func (s *session) SetMode(mode string) error {
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
	return nil // applied from the next turn; approvals are still enforced by the hub policy
}

func (s *session) Cancel() error {
	s.mu.Lock()
	turn := s.turnID
	s.mu.Unlock()
	if turn == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.c.call(ctx, "turn/interrupt", map[string]any{"threadId": s.threadID, "turnId": turn})
	return err
}

func (s *session) Close() error { s.c.p.Close(); return nil }

func (s *session) Decide(approvalID string, d core.Decision) error {
	s.mu.Lock()
	pr, ok := s.pending[approvalID]
	delete(s.pending, approvalID)
	s.mu.Unlock()
	if !ok {
		return core.ErrNoApproval
	}
	if pr.kind == "permissions" {
		granted := map[string]any{}
		if d.Allow {
			var p struct {
				Permissions json.RawMessage `json:"permissions"`
			}
			if json.Unmarshal(pr.params, &p) == nil && len(p.Permissions) > 0 {
				_ = json.Unmarshal(p.Permissions, &granted)
			}
		}
		scope := "turn"
		if d.Scope == "session" {
			scope = "session"
		}
		return s.c.respond(pr.rpcID, map[string]any{"permissions": granted, "scope": scope})
	}
	decision := "decline"
	if d.Allow {
		decision = "accept"
		if d.Scope == "session" {
			decision = "acceptForSession"
		}
	}
	return s.c.respond(pr.rpcID, map[string]any{"decision": decision})
}

func classify(text string) string {
	l := strings.ToLower(text)
	switch {
	case strings.Contains(l, "401") || strings.Contains(l, "unauthorized") || strings.Contains(l, "not logged in") || strings.Contains(l, "sign in"):
		return "auth"
	case strings.Contains(l, "rate limit") || strings.Contains(l, "usage limit") || strings.Contains(l, "quota") || strings.Contains(l, "429"):
		return "quota_exceeded"
	}
	return "other"
}

type item struct {
	Type             string          `json:"type"`
	ID               string          `json:"id"`
	Command          string          `json:"command"`
	Cwd              string          `json:"cwd"`
	AggregatedOutput string          `json:"aggregatedOutput"`
	Status           string          `json:"status"`
	Tool             string          `json:"tool"`
	Server           string          `json:"server"`
	Arguments        json.RawMessage `json:"arguments"`
	Query            string          `json:"query"`
	Changes          []struct {
		Path string `json:"path"`
		Diff string `json:"diff"`
	} `json:"changes"`
}

func (s *session) onMsg(m rpcMsg) {
	if len(m.ID) > 0 && m.Method != "" {
		s.onServerRequest(m)
		return
	}
	switch m.Method {
	case "turn/started":
		var p struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(m.Params, &p) == nil && p.Turn.ID != "" {
			s.mu.Lock()
			s.turnID = p.Turn.ID
			s.mu.Unlock()
		}
	case "item/agentMessage/delta":
		var p struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(m.Params, &p) == nil && p.Delta != "" {
			s.emit(core.Event{Type: core.EvTextDelta, Text: p.Delta})
		}
	case "item/started", "item/completed":
		var p struct {
			Item item `json:"item"`
		}
		if json.Unmarshal(m.Params, &p) != nil {
			return
		}
		s.onItem(m.Method == "item/started", p.Item)
	case "thread/tokenUsage/updated":
		var p struct {
			TokenUsage struct {
				Last struct {
					In  int `json:"inputTokens"`
					Out int `json:"outputTokens"`
				} `json:"last"`
			} `json:"tokenUsage"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			s.mu.Lock()
			s.lastTok = &core.Usage{InputTokens: p.TokenUsage.Last.In, OutputTokens: p.TokenUsage.Last.Out}
			s.mu.Unlock()
		}
	case "turn/completed":
		var p struct {
			Turn struct {
				Status string `json:"status"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"turn"`
		}
		_ = json.Unmarshal(m.Params, &p)
		s.mu.Lock()
		usage := s.lastTok
		s.mu.Unlock()
		switch {
		case p.Turn.Status == "interrupted":
			s.emit(core.Event{Type: core.EvError, Err: &core.ErrInfo{Kind: "cancelled", Message: "interrupted"}})
			s.emit(core.Event{Type: core.EvTurnDone})
		case p.Turn.Error != nil || p.Turn.Status == "failed":
			msg := "turn failed"
			if p.Turn.Error != nil {
				msg = p.Turn.Error.Message
			}
			kind := classify(msg)
			s.emit(core.Event{Type: core.EvError, Err: &core.ErrInfo{Kind: kind, Message: msg}})
			if kind == "other" {
				s.emit(core.Event{Type: core.EvTurnDone})
			}
		default:
			s.emit(core.Event{Type: core.EvTurnDone, Usage: usage})
		}
	}
}

func (s *session) onItem(started bool, it item) {
	switch it.Type {
	case "commandExecution":
		if started {
			s.emit(core.Event{Type: core.EvToolCall, Tool: &core.ToolCall{ID: it.ID, Name: "Bash", Args: map[string]any{"command": it.Command, "cwd": it.Cwd}}})
		} else {
			s.emit(core.Event{Type: core.EvToolResult, Tool: &core.ToolCall{ID: it.ID, Output: it.AggregatedOutput}})
		}
	case "fileChange":
		if started {
			var paths []string
			for _, c := range it.Changes {
				paths = append(paths, c.Path)
			}
			s.emit(core.Event{Type: core.EvToolCall, Tool: &core.ToolCall{ID: it.ID, Name: "Edit", Args: map[string]any{"file_path": strings.Join(paths, ", ")}}})
			return
		}
		for _, c := range it.Changes {
			s.emit(core.Event{Type: core.EvDiff, Diff: &core.Diff{File: c.Path, Patch: c.Diff}})
		}
		s.emit(core.Event{Type: core.EvToolResult, Tool: &core.ToolCall{ID: it.ID, Output: it.Status}})
	case "mcpToolCall", "dynamicToolCall":
		if started {
			var args map[string]any
			_ = json.Unmarshal(it.Arguments, &args)
			s.emit(core.Event{Type: core.EvToolCall, Tool: &core.ToolCall{ID: it.ID, Name: strings.Trim(it.Server+"."+it.Tool, "."), Args: args}})
		} else {
			s.emit(core.Event{Type: core.EvToolResult, Tool: &core.ToolCall{ID: it.ID, Output: it.Status}})
		}
	case "webSearch":
		if started {
			s.emit(core.Event{Type: core.EvToolCall, Tool: &core.ToolCall{ID: it.ID, Name: "WebSearch", Args: map[string]any{"pattern": it.Query}}})
		}
	}
}

func (s *session) onServerRequest(m rpcMsg) {
	id := "cx-" + strings.Trim(string(m.ID), `"`)
	var p struct {
		Command string          `json:"command"`
		Cwd     string          `json:"cwd"`
		Reason  string          `json:"reason"`
		Root    string          `json:"grantRoot"`
		Perms   json.RawMessage `json:"permissions"`
	}
	_ = json.Unmarshal(m.Params, &p)
	switch m.Method {
	case "item/commandExecution/requestApproval":
		s.addPending(id, pendingReq{rpcID: m.ID, kind: "command", params: m.Params})
		s.emit(core.Event{Type: core.EvApprovalRequest, Approval: &core.ApprovalRequest{ID: id, Tool: "Bash", Risk: "high", Title: p.Reason,
			Args: map[string]any{"command": p.Command, "cwd": p.Cwd, "reason": p.Reason}}})
	case "item/fileChange/requestApproval":
		s.addPending(id, pendingReq{rpcID: m.ID, kind: "file", params: m.Params})
		args := map[string]any{"reason": p.Reason}
		if p.Root != "" {
			args["file_path"] = p.Root
		}
		s.emit(core.Event{Type: core.EvApprovalRequest, Approval: &core.ApprovalRequest{ID: id, Tool: "Edit", Risk: "high", Title: p.Reason, Args: args}})
	case "item/permissions/requestApproval":
		s.addPending(id, pendingReq{rpcID: m.ID, kind: "permissions", params: m.Params})
		var perms map[string]any
		_ = json.Unmarshal(p.Perms, &perms)
		s.emit(core.Event{Type: core.EvApprovalRequest, Approval: &core.ApprovalRequest{ID: id, Tool: "Permissions", Risk: "high", Title: p.Reason, Args: map[string]any{"reason": p.Reason, "permissions": perms}}})
	default:
		_ = s.c.respondErr(m.ID, -32601, "not supported by agent-hub: "+m.Method)
	}
}

func (s *session) addPending(id string, pr pendingReq) {
	s.mu.Lock()
	s.pending[id] = pr
	s.mu.Unlock()
}

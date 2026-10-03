// Package claude adapts Claude Code's stream-json protocol (including the
// stdio permission-prompt control channel) to the engine-agnostic core.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nchungdev/agent-hub/internal/adapters/proc"
	"github.com/nchungdev/agent-hub/internal/core"
)

type Engine struct{ Bin string }

func New(bin string) *Engine {
	if bin == "" {
		if p, err := exec.LookPath("claude"); err == nil {
			bin = p
		}
	}
	return &Engine{Bin: bin}
}

func (e *Engine) ID() string { return "claude" }
func (e *Engine) Capabilities() core.Capabilities {
	return core.Capabilities{Streaming: true, Resume: true, PermissionPrompts: true, PlanMode: true,
		PermissionModes: []string{"ask", "plan", "accept-edits", "bypass"}}
}
func (e *Engine) Models(context.Context) ([]core.Model, error) {
	return []core.Model{
		{ID: "claude-sonnet-5-5", Name: "Claude Sonnet 5.5", Tier: "smart"},
		{ID: "claude-opus-5-5", Name: "Claude Opus 5.5", Tier: "thinking"},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Tier: "fast"},
	}, nil
}

// Commands asks a short-lived CLI process for its slash commands and skills using the
// SDK `initialize` control request (no model call is made).
func (e *Engine) Commands(ctx context.Context) ([]core.Command, error) {
	if e.Bin == "" {
		return nil, fmt.Errorf("claude binary not found")
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	home, _ := os.UserHomeDir()
	p, err := proc.Start(cctx, proc.Options{Bin: e.Bin, Args: []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose"}, Dir: home, Env: []string{"TERM=dumb", "NO_COLOR=1"}})
	if err != nil {
		return nil, err
	}
	defer p.Kill()
	if err := p.WriteLine([]byte(`{"type":"control_request","request_id":"hub-init","request":{"subtype":"initialize"}}`)); err != nil {
		return nil, err
	}
	type result struct {
		cmds []core.Command
		err  error
	}
	done := make(chan result, 1)
	go func() {
		for {
			line, err := p.Stdout.ReadBytes('\n')
			if len(line) > 0 {
				var m struct {
					Type     string `json:"type"`
					Response struct {
						RequestID string `json:"request_id"`
						Response  struct {
							Commands []struct {
								Name         string `json:"name"`
								Description  string `json:"description"`
								ArgumentHint string `json:"argumentHint"`
							} `json:"commands"`
						} `json:"response"`
					} `json:"response"`
				}
				if json.Unmarshal(line, &m) == nil && m.Type == "control_response" && m.Response.RequestID == "hub-init" {
					var out []core.Command
					for _, c := range m.Response.Response.Commands {
						out = append(out, core.Command{Name: c.Name, Description: c.Description, ArgHint: c.ArgumentHint, Kind: "command"})
					}
					done <- result{cmds: out}
					return
				}
			}
			if err != nil {
				done <- result{err: fmt.Errorf("claude exited before replying: %s", p.StderrTail())}
				return
			}
		}
	}()
	select {
	case r := <-done:
		return r.cmds, r.err
	case <-cctx.Done():
		return nil, cctx.Err()
	}
}

// SummaryModel is the cheapest Claude model (the "fast" tier).
func (e *Engine) SummaryModel() string  { return "claude-haiku-4-5" }
func (e *Engine) SummaryEngine() string { return "claude" }

// Summarize runs a lean one-shot completion: no tools, plugins, MCP servers, skills or saved session,
// so the call costs little more than the prompt itself.
func (e *Engine) Summarize(ctx context.Context, system, prompt string) (string, core.Usage, error) {
	var usage core.Usage
	if e.Bin == "" {
		return "", usage, fmt.Errorf("claude binary not found")
	}
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, e.Bin, "-p", "--model", e.SummaryModel(), "--output-format", "json",
		"--no-session-persistence", "--tools", "", "--disable-slash-commands", "--strict-mcp-config", "--system-prompt", system)
	home, _ := os.UserHomeDir()
	cmd.Dir = home
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return "", usage, fmt.Errorf("claude summarize: %w", err)
	}
	var r struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
		Usage   struct {
			In  int `json:"input_tokens"`
			Out int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return "", usage, fmt.Errorf("claude summarize: bad output: %w", err)
	}
	usage = core.Usage{InputTokens: r.Usage.In, OutputTokens: r.Usage.Out}
	if r.IsError || strings.TrimSpace(r.Result) == "" {
		return "", usage, fmt.Errorf("claude summarize failed: %s", r.Result)
	}
	return strings.TrimSpace(r.Result), usage, nil
}

// LoginCommand lets the hub run the subscription login flow from the GUI.
func (e *Engine) LoginCommand() (string, []string) { return e.Bin, []string{"auth", "login"} }

// Status runs `claude auth status` (JSON) so the GUI can warn before a message is sent.
func (e *Engine) Status(ctx context.Context) core.AuthStatus {
	if e.Bin == "" {
		return core.AuthStatus{Installed: false, Known: true, Detail: "claude CLI not found", LoginHint: "npm install -g @anthropic-ai/claude-code"}
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, e.Bin, "auth", "status").Output()
	st := core.AuthStatus{Installed: true, LoginHint: "On the NAS run: claude auth login"}
	if err != nil && len(out) == 0 {
		st.Detail = "could not query auth status"
		return st
	}
	var r struct {
		LoggedIn   bool   `json:"loggedIn"`
		AuthMethod string `json:"authMethod"`
		Email      string `json:"email"`
	}
	if json.Unmarshal(out, &r) != nil {
		st.Detail = "unrecognised auth status output"
		return st
	}
	st.Known, st.LoggedIn = true, r.LoggedIn
	if r.LoggedIn {
		st.Detail = strings.TrimSpace(r.AuthMethod + " " + r.Email)
	} else {
		st.Detail = "not logged in"
	}
	return st
}

// cliMode maps hub modes to Claude permission modes. "ask" and "bypass" both
// run Claude in default mode: the hub PolicyEngine answers every prompt, so
// bypass is a hub decision, not a CLI flag.
func cliMode(m string) string {
	switch m {
	case "plan":
		return "plan"
	case "accept-edits":
		return "acceptEdits"
	}
	return "default"
}

func (e *Engine) Start(ctx context.Context, o core.StartOpts) (core.Session, error) {
	if e.Bin == "" {
		return nil, fmt.Errorf("claude binary not found in PATH")
	}
	id := o.ResumeID
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--include-partial-messages", "--permission-prompt-tool", "stdio", "--permission-mode", cliMode(o.Mode)}
	if id != "" {
		args = append(args, "--resume", id)
	} else {
		id = uuid.NewString()
		args = append(args, "--session-id", id)
	}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	if o.Effort != "" {
		args = append(args, "--effort", strings.ToLower(o.Effort))
	}
	p, err := proc.Start(ctx, proc.Options{Bin: e.Bin, Args: args, Dir: o.Workspace, Env: []string{"TERM=dumb", "NO_COLOR=1"}})
	if err != nil {
		return nil, err
	}
	s := &session{p: p, id: id, events: make(chan core.Event, 256), pending: map[string]json.RawMessage{}}
	go s.read()
	return s, nil
}

type session struct {
	p      *proc.Proc
	id     string
	events chan core.Event

	mu      sync.Mutex
	pending map[string]json.RawMessage // control request id -> tool input
	sawText bool                       // partial text deltas seen in current turn
	reqN    int
}

func (s *session) EngineSessionID() string   { return s.id }
func (s *session) Events() <-chan core.Event { return s.events }

func (s *session) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.p.WriteLine(b)
}

func (s *session) Send(_ context.Context, in core.UserInput) error {
	text := in.Text
	if in.Preamble != "" {
		text = "<context>\n" + in.Preamble + "\n</context>\n\n" + text
	}
	s.mu.Lock()
	s.sawText = false
	s.mu.Unlock()
	return s.send(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}})
}

func (s *session) nextReq() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqN++
	return fmt.Sprintf("hub-%d", s.reqN)
}

func (s *session) Decide(approvalID string, d core.Decision) error {
	s.mu.Lock()
	input, ok := s.pending[approvalID]
	delete(s.pending, approvalID)
	s.mu.Unlock()
	if !ok {
		return core.ErrNoApproval
	}
	body := map[string]any{}
	if d.Allow {
		var in any
		_ = json.Unmarshal(input, &in)
		body = map[string]any{"behavior": "allow", "updatedInput": in}
	} else {
		msg := d.Reason
		if msg == "" {
			msg = "Denied by user"
		}
		body = map[string]any{"behavior": "deny", "message": msg}
	}
	return s.send(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": approvalID, "response": body}})
}

func (s *session) SetMode(mode string) error {
	return s.send(map[string]any{"type": "control_request", "request_id": s.nextReq(), "request": map[string]any{"subtype": "set_permission_mode", "mode": cliMode(mode)}})
}

func (s *session) Cancel() error {
	return s.send(map[string]any{"type": "control_request", "request_id": s.nextReq(), "request": map[string]any{"subtype": "interrupt"}})
}

func (s *session) Close() error { s.p.Close(); return nil }

func (s *session) emit(ev core.Event) { ev.Time = time.Now(); s.events <- ev }

func riskOf(tool string) string {
	switch tool {
	case "Bash", "Write", "Edit", "NotebookEdit":
		return "high"
	case "WebFetch", "WebSearch":
		return "medium"
	}
	return "low"
}

func (s *session) read() {
	defer close(s.events)
	for {
		line, err := s.p.Stdout.ReadBytes('\n')
		if len(line) > 0 {
			s.handle(line)
		}
		if err != nil {
			if err != io.EOF {
				s.emit(core.Event{Type: core.EvError, Err: &core.ErrInfo{Kind: "protocol", Message: err.Error()}})
			}
			<-s.p.Done()
			return
		}
	}
}

type msg struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
	Event     json.RawMessage `json:"event"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	IsError bool            `json:"is_error"`
	Result  string          `json:"result"`
	Usage   json.RawMessage `json:"usage"`
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

func contentText(raw json.RawMessage) string {
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}
	var parts []block
	if json.Unmarshal(raw, &parts) == nil {
		var sb strings.Builder
		for _, p := range parts {
			sb.WriteString(p.Text)
		}
		return sb.String()
	}
	return ""
}

func classify(text string) string {
	l := strings.ToLower(text)
	switch {
	case strings.Contains(l, "not logged in") || strings.Contains(l, "/login") || strings.Contains(l, "invalid api key") || strings.Contains(l, "authentication"):
		return "auth"
	case strings.Contains(l, "rate limit") || strings.Contains(l, "usage limit") || strings.Contains(l, "quota") || strings.Contains(l, "overloaded"):
		return "quota_exceeded"
	}
	return "other"
}

func (s *session) handle(line []byte) {
	var m msg
	if err := json.Unmarshal(line, &m); err != nil {
		return
	}
	switch m.Type {
	case "stream_event":
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if json.Unmarshal(m.Event, &ev) == nil && ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
			s.mu.Lock()
			s.sawText = true
			s.mu.Unlock()
			s.emit(core.Event{Type: core.EvTextDelta, Text: ev.Delta.Text})
		}
	case "assistant":
		var blocks []block
		if json.Unmarshal(m.Message.Content, &blocks) != nil {
			return
		}
		s.mu.Lock()
		saw := s.sawText
		s.mu.Unlock()
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if !saw && b.Text != "" {
					s.emit(core.Event{Type: core.EvTextDelta, Text: b.Text})
				}
			case "tool_use":
				var args map[string]any
				_ = json.Unmarshal(b.Input, &args)
				s.emit(core.Event{Type: core.EvToolCall, Tool: &core.ToolCall{ID: b.ID, Name: b.Name, Args: args}})
			}
		}
	case "user":
		var blocks []block
		if json.Unmarshal(m.Message.Content, &blocks) != nil {
			return
		}
		for _, b := range blocks {
			if b.Type == "tool_result" {
				s.emit(core.Event{Type: core.EvToolResult, Tool: &core.ToolCall{ID: b.ToolUseID, Output: contentText(b.Content)}})
			}
		}
	case "control_request":
		var r struct {
			Subtype   string          `json:"subtype"`
			ToolName  string          `json:"tool_name"`
			Input     json.RawMessage `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
			Title     string          `json:"title"`
		}
		if json.Unmarshal(m.Request, &r) != nil || r.Subtype != "can_use_tool" {
			return
		}
		var args map[string]any
		_ = json.Unmarshal(r.Input, &args)
		s.mu.Lock()
		s.pending[m.RequestID] = r.Input
		s.mu.Unlock()
		s.emit(core.Event{Type: core.EvApprovalRequest, Approval: &core.ApprovalRequest{ID: m.RequestID, Tool: r.ToolName, Args: args, Risk: riskOf(r.ToolName), Title: r.Title}})
	case "result":
		if m.IsError {
			kind := classify(m.Result)
			s.emit(core.Event{Type: core.EvError, Err: &core.ErrInfo{Kind: kind, Message: m.Result}})
			if kind == "other" {
				s.emit(core.Event{Type: core.EvTurnDone})
			}
			return
		}
		ev := core.Event{Type: core.EvTurnDone}
		var u struct {
			In  int `json:"input_tokens"`
			Out int `json:"output_tokens"`
		}
		if json.Unmarshal(m.Usage, &u) == nil && (u.In > 0 || u.Out > 0) {
			ev.Usage = &core.Usage{InputTokens: u.In, OutputTokens: u.Out}
		}
		s.emit(ev)
	}
}

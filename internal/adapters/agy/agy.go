// Package agy adapts the Antigravity CLI's print-mode stream-json protocol.
// The CLI's stdin accepts only `user` events (no approval channel), so this
// engine reports PermissionPrompts=false: safety comes from the permission
// mode chosen at start (plan | accept-edits | bypass), never from prompting.
package agy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nchungdev/agent-hub/internal/adapters/proc"
	"github.com/nchungdev/agent-hub/internal/core"
)

type Engine struct{ Bin string }

func New(bin string) *Engine {
	if bin == "" {
		if p, err := exec.LookPath("agy"); err == nil {
			bin = p
		}
	}
	return &Engine{Bin: bin}
}

func (e *Engine) ID() string { return "agy" }
func (e *Engine) Capabilities() core.Capabilities {
	return core.Capabilities{Streaming: true, Resume: true, PermissionPrompts: false, PlanMode: true, ModelListing: true,
		PermissionModes: []string{"plan", "accept-edits", "bypass"}, ModeRequiresRestart: true}
}

// Status: the CLI has no auth-status command, so only installation is checked.
func (e *Engine) Status(context.Context) core.AuthStatus {
	if e.Bin == "" {
		return core.AuthStatus{Installed: false, Known: true, Detail: "agy CLI not found"}
	}
	return core.AuthStatus{Installed: true, Known: false}
}

func (e *Engine) Models(ctx context.Context) ([]core.Model, error) {
	fallback := []core.Model{{ID: "gemini-3.8-flash-medium", Name: "Gemini 3.8 Flash (Medium)"}}
	if e.Bin == "" {
		return fallback, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, e.Bin, "models").Output()
	if err != nil {
		return fallback, nil
	}
	var ms []core.Model
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(parts) == 2 && parts[0] != "" {
			ms = append(ms, core.Model{ID: parts[0], Name: parts[1]})
		}
	}
	if len(ms) == 0 {
		return fallback, nil
	}
	return ms, nil
}

// Commands lists the user's skills (the CLI has no command-listing API): every
// <dir>/<skill>/SKILL.md under the Antigravity skill directories.
func (e *Engine) Commands(context.Context) ([]core.Command, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var out []core.Command
	seen := map[string]bool{}
	for _, dir := range []string{".gemini/skills", ".gemini/antigravity-cli/skills", ".agents/skills"} {
		entries, _ := os.ReadDir(filepath.Join(home, dir))
		for _, en := range entries {
			b, err := os.ReadFile(filepath.Join(home, dir, en.Name(), "SKILL.md"))
			if err != nil {
				continue
			}
			name, desc := skillMeta(string(b), en.Name())
			if !seen[name] {
				seen[name] = true
				out = append(out, core.Command{Name: name, Description: desc, Kind: "skill"})
			}
		}
	}
	return out, nil
}

// skillMeta reads `name:` and `description:` from SKILL.md front matter.
func skillMeta(doc, fallback string) (name, desc string) {
	name = fallback
	if !strings.HasPrefix(doc, "---") {
		return
	}
	end := strings.Index(doc[3:], "\n---")
	if end < 0 {
		return
	}
	for _, l := range strings.Split(doc[3:3+end], "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "name":
			if v != "" {
				name = v
			}
		case "description":
			desc = v
		}
	}
	return
}

func effortBaked(model string) bool {
	for _, e := range []string{"-low", "-medium", "-high", "-xhigh", "-max"} {
		if strings.HasSuffix(model, e) {
			return true
		}
	}
	return false
}

func cliMode(m string) (mode string, bypass bool) {
	switch m {
	case "plan":
		return "plan", false
	case "bypass":
		return "accept-edits", true
	}
	return "accept-edits", false
}

type session struct {
	e    *Engine
	opts core.StartOpts

	mu      sync.Mutex
	p       *proc.Proc
	gen     int
	convID  string
	initCh  chan struct{}
	events  chan core.Event
	closed  bool
	inTurn  bool
	sawText bool
	mode    string
}

func (e *Engine) Start(ctx context.Context, o core.StartOpts) (core.Session, error) {
	if e.Bin == "" {
		return nil, fmt.Errorf("agy binary not found in PATH")
	}
	s := &session{e: e, opts: o, events: make(chan core.Event, 256), mode: o.Mode, convID: o.ResumeID}
	if err := s.spawn(ctx, o.ResumeID); err != nil {
		return nil, err
	}
	return s, nil
}

// spawn starts the CLI process and waits for its init event (which carries the conversation id).
func (s *session) spawn(ctx context.Context, conv string) error {
	mode, bypass := cliMode(s.mode)
	args := []string{"-p=", "--input-format", "stream-json", "--output-format", "stream-json", "--mode", mode}
	if bypass {
		args = append(args, "--dangerously-skip-permissions")
	}
	if s.opts.Model != "" {
		args = append(args, "--model", s.opts.Model)
	}
	// `agy models` lists variants with the effort baked into the id (…-high); the CLI rejects --effort
	// together with such a model, so only pass --effort for ids without that suffix.
	if s.opts.Effort != "" && !effortBaked(s.opts.Model) {
		args = append(args, "--effort", strings.ToLower(s.opts.Effort))
	}
	if conv != "" {
		args = append(args, "--conversation", conv)
	}
	p, err := proc.Start(context.Background(), proc.Options{Bin: s.e.Bin, Args: args, Dir: s.opts.Workspace, Env: []string{"TERM=dumb", "NO_COLOR=1"}})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.gen++
	gen := s.gen
	s.p = p
	s.initCh = make(chan struct{})
	initCh := s.initCh
	s.mu.Unlock()
	go s.read(p, gen)
	select {
	case <-initCh:
		return nil
	case <-p.Done():
		return fmt.Errorf("agy exited before init: %s", p.StderrTail())
	case <-time.After(30 * time.Second):
		p.Kill()
		return fmt.Errorf("agy did not initialise in time")
	case <-ctx.Done():
		p.Kill()
		return ctx.Err()
	}
}

func (s *session) EngineSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.convID
}
func (s *session) Events() <-chan core.Event { return s.events }

func (s *session) emit(ev core.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	ev.Time = time.Now()
	s.events <- ev
}

func (s *session) Send(_ context.Context, in core.UserInput) error {
	text := in.Text
	if in.Preamble != "" {
		text = "<context>\n" + in.Preamble + "\n</context>\n\n" + text
	}
	s.mu.Lock()
	p := s.p
	s.inTurn, s.sawText = true, false
	s.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"event": "user", "message": map[string]any{"content": text}})
	return p.WriteLine(b)
}

func (s *session) Decide(string, core.Decision) error { return core.ErrNoApproval }

// SetMode records the mode; it takes effect when the manager restarts the session.
func (s *session) SetMode(mode string) error {
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
	return nil
}

// Cancel has no protocol support, so the process is killed and respawned on the same conversation.
func (s *session) Cancel() error {
	s.mu.Lock()
	if !s.inTurn || s.closed {
		s.mu.Unlock()
		return nil
	}
	s.inTurn = false
	p := s.p
	conv := s.convID
	s.gen++ // the old reader must not report this exit as a crash
	s.mu.Unlock()
	p.Kill()
	if err := s.spawn(context.Background(), conv); err != nil {
		s.emit(core.Event{Type: core.EvError, Err: &core.ErrInfo{Kind: "crashed", Message: "could not restart after cancel: " + err.Error()}})
		s.closeEvents()
		return err
	}
	s.emit(core.Event{Type: core.EvError, Err: &core.ErrInfo{Kind: "cancelled", Message: "cancelled"}})
	s.emit(core.Event{Type: core.EvTurnDone})
	return nil
}

func (s *session) closeEvents() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.events)
	}
}

func (s *session) Close() error {
	s.mu.Lock()
	p := s.p
	s.gen++
	s.mu.Unlock()
	if p != nil {
		p.Close()
	}
	s.closeEvents()
	return nil
}

func classify(text string) string {
	l := strings.ToLower(text)
	switch {
	case strings.Contains(l, "quota") || strings.Contains(l, "resource_exhausted") || strings.Contains(l, "429") || strings.Contains(l, "rate limit"):
		return "quota_exceeded"
	case strings.Contains(l, "not authenticated") || strings.Contains(l, "unauthenticated") || strings.Contains(l, "sign in") || strings.Contains(l, "login"):
		return "auth"
	}
	return "other"
}

type line struct {
	Event          string `json:"event"`
	ConversationID string `json:"conversation_id"`
	StepUpdate     *struct {
		TextDelta string `json:"text_delta"`
	} `json:"step_update"`
	Result *struct {
		ConversationID string `json:"conversation_id"`
		Status         string `json:"status"`
		Response       string `json:"response"`
		Error          string `json:"error"`
		Usage          struct {
			In  int `json:"input_tokens"`
			Out int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"result"`
}

func (s *session) read(p *proc.Proc, gen int) {
	for {
		b, err := p.Stdout.ReadBytes('\n')
		if len(b) > 0 {
			s.handle(gen, b)
		}
		if err != nil {
			if err != io.EOF {
				_ = err
			}
			<-p.Done()
			s.mu.Lock()
			stale := s.gen != gen || s.closed
			s.mu.Unlock()
			if !stale {
				s.closeEvents() // unexpected exit: the manager reports a crash
			}
			return
		}
	}
}

func (s *session) handle(gen int, b []byte) {
	var l line
	if json.Unmarshal(b, &l) != nil {
		return
	}
	s.mu.Lock()
	if s.gen != gen {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	switch l.Event {
	case "init":
		s.mu.Lock()
		if l.ConversationID != "" {
			s.convID = l.ConversationID
		}
		if s.initCh != nil {
			select {
			case <-s.initCh:
			default:
				close(s.initCh)
			}
		}
		s.mu.Unlock()
	case "step_update":
		if l.StepUpdate != nil && l.StepUpdate.TextDelta != "" {
			s.mu.Lock()
			s.sawText = true
			s.mu.Unlock()
			s.emit(core.Event{Type: core.EvTextDelta, Text: l.StepUpdate.TextDelta})
		}
	case "result":
		if l.Result == nil {
			return
		}
		s.mu.Lock()
		saw := s.sawText
		s.inTurn = false
		s.mu.Unlock()
		if l.Result.Status == "ERROR" || l.Result.Error != "" {
			kind := classify(l.Result.Error)
			s.emit(core.Event{Type: core.EvError, Err: &core.ErrInfo{Kind: kind, Message: l.Result.Error}})
			if kind == "other" {
				s.emit(core.Event{Type: core.EvTurnDone})
			}
			return
		}
		if !saw && l.Result.Response != "" {
			s.emit(core.Event{Type: core.EvTextDelta, Text: l.Result.Response})
		}
		ev := core.Event{Type: core.EvTurnDone}
		if l.Result.Usage.In > 0 || l.Result.Usage.Out > 0 {
			ev.Usage = &core.Usage{InputTokens: l.Result.Usage.In, OutputTokens: l.Result.Usage.Out}
		}
		s.emit(ev)
	}
}

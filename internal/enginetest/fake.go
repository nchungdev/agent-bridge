// Package enginetest provides a scriptable in-process Engine used by contract
// and manager tests. Magic words in the user text drive behaviour:
// [approve] asks for approval, [slow] waits until cancelled or 200ms, [crash]
// kills the session abruptly.
package enginetest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nchungdev/agent-hub/internal/core"
)

type Engine struct {
	Name string
	Caps core.Capabilities

	mu           sync.Mutex
	started      int
	LastPreamble string
	LastStart    core.StartOpts
}

func New(name string) *Engine {
	return &Engine{Name: name, Caps: core.Capabilities{Streaming: true, Resume: true, PermissionPrompts: true, PermissionModes: []string{"ask", "plan", "bypass"}}}
}

func (e *Engine) ID() string                      { return e.Name }
func (e *Engine) Capabilities() core.Capabilities { return e.Caps }
func (e *Engine) Models(context.Context) ([]core.Model, error) {
	return []core.Model{{ID: e.Name + "-model", Name: e.Name}}, nil
}
func (e *Engine) Starts() int      { e.mu.Lock(); defer e.mu.Unlock(); return e.started }
func (e *Engine) Preamble() string { e.mu.Lock(); defer e.mu.Unlock(); return e.LastPreamble }

func (e *Engine) Start(_ context.Context, o core.StartOpts) (core.Session, error) {
	e.mu.Lock()
	e.started++
	e.LastStart = o
	id := o.ResumeID
	if id == "" {
		id = fmt.Sprintf("%s-sess-%d", e.Name, e.started)
	}
	e.mu.Unlock()
	return &session{eng: e, id: id, events: make(chan core.Event, 256), decide: make(chan core.Decision, 1)}, nil
}

type session struct {
	eng    *Engine
	id     string
	events chan core.Event
	decide chan core.Decision

	mu     sync.Mutex
	closed bool
	cancel chan struct{}
	mode   string
}

func (s *session) EngineSessionID() string   { return s.id }
func (s *session) Events() <-chan core.Event { return s.events }
func (s *session) SetMode(m string) error    { s.mu.Lock(); s.mode = m; s.mu.Unlock(); return nil }

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
	s.eng.mu.Lock()
	s.eng.LastPreamble = in.Preamble
	s.eng.mu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return core.ErrNotLive
	}
	s.cancel = make(chan struct{})
	cancel := s.cancel
	s.mu.Unlock()
	go s.turn(in.Text, cancel)
	return nil
}

func (s *session) turn(text string, cancel chan struct{}) {
	if strings.Contains(text, "[crash]") {
		s.Close()
		return
	}
	if strings.Contains(text, "[quota]") {
		s.emit(core.Event{Type: core.EvError, Err: &core.ErrInfo{Kind: "quota_exceeded", Message: "limit reached"}})
		return
	}
	if strings.Contains(text, "[slow]") {
		select {
		case <-cancel:
			s.emit(core.Event{Type: core.EvError, Err: &core.ErrInfo{Kind: "cancelled", Message: "cancelled"}})
			s.emit(core.Event{Type: core.EvTurnDone})
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
	if strings.Contains(text, "[approve]") {
		s.emit(core.Event{Type: core.EvApprovalRequest, Approval: &core.ApprovalRequest{ID: fmt.Sprintf("%s-ap-%d", s.id, time.Now().UnixNano()), Tool: "Bash", Args: map[string]any{"command": "rm -rf /tmp/x"}, Risk: "high"}})
		select {
		case d := <-s.decide:
			if d.Allow {
				s.emit(core.Event{Type: core.EvTextDelta, Text: "approved "})
			} else {
				s.emit(core.Event{Type: core.EvTextDelta, Text: "denied "})
			}
		case <-cancel:
			s.emit(core.Event{Type: core.EvTurnDone})
			return
		}
	}
	for _, w := range strings.Fields("echo: " + text) {
		s.emit(core.Event{Type: core.EvTextDelta, Text: w + " "})
	}
	s.emit(core.Event{Type: core.EvTurnDone, Usage: &core.Usage{InputTokens: 1, OutputTokens: 1}})
}

func (s *session) Decide(_ string, d core.Decision) error {
	select {
	case s.decide <- d:
		return nil
	default:
		return core.ErrNoApproval
	}
}

func (s *session) Cancel() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		select {
		case <-s.cancel:
		default:
			close(s.cancel)
		}
	}
	return nil
}

func (s *session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.events)
	}
	return nil
}

// FakeSummarizer records prompts and returns a canned summary.
type FakeSummarizer struct {
	Out     string
	Err     error
	Name    string
	mu      sync.Mutex
	Prompts []string
}

func (f *FakeSummarizer) SummaryModel() string  { return f.Name }
func (f *FakeSummarizer) SummaryEngine() string { return f.Name }
func (f *FakeSummarizer) Summarize(_ context.Context, _, prompt string) (string, core.Usage, error) {
	f.mu.Lock()
	f.Prompts = append(f.Prompts, prompt)
	f.mu.Unlock()
	return f.Out, core.Usage{InputTokens: 10, OutputTokens: 5}, f.Err
}
func (f *FakeSummarizer) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.Prompts...)
}

// Package core holds the engine-agnostic domain: events, engine/session
// contracts, capabilities and the session state machine. It imports only the
// standard library so adapters and the application layer both depend on it,
// never the other way round.
package core

import (
	"context"
	"errors"
	"time"
)

type EventType string

const (
	EvTextDelta        EventType = "text_delta"
	EvUserMessage      EventType = "user_message"
	EvToolCall         EventType = "tool_call"
	EvToolResult       EventType = "tool_result"
	EvApprovalRequest  EventType = "approval_request"
	EvApprovalResolved EventType = "approval_resolved"
	EvDiff             EventType = "diff"
	EvUsage            EventType = "usage"
	EvError            EventType = "error"
	EvTurnDone         EventType = "turn_done"
	EvEngineSwitch     EventType = "engine_switch"
	EvStateChange      EventType = "state_change"
)

type ToolCall struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Args   map[string]any `json:"args,omitempty"`
	Output string         `json:"output,omitempty"`
}

type ApprovalRequest struct {
	ID    string         `json:"id"`
	Tool  string         `json:"tool"`
	Args  map[string]any `json:"args,omitempty"`
	Risk  string         `json:"risk,omitempty"` // low | medium | high
	Title string         `json:"title,omitempty"`
}

type Diff struct {
	File  string `json:"file"`
	Patch string `json:"patch"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ErrInfo struct {
	Kind    string `json:"kind"` // quota_exceeded | auth | crashed | protocol | cancelled | other
	Message string `json:"message"`
}

// Event is the single normalised event every adapter emits. Adapters translate
// their CLI's private output into this shape (anti-corruption layer).
type Event struct {
	Seq      int64            `json:"seq,omitempty"`
	ConvID   string           `json:"conv_id,omitempty"`
	Engine   string           `json:"engine,omitempty"`
	Model    string           `json:"model,omitempty"`
	Type     EventType        `json:"type"`
	Time     time.Time        `json:"time"`
	Text     string           `json:"text,omitempty"`
	Tool     *ToolCall        `json:"tool,omitempty"`
	Approval *ApprovalRequest `json:"approval,omitempty"`
	Diff     *Diff            `json:"diff,omitempty"`
	Usage    *Usage           `json:"usage,omitempty"`
	Err      *ErrInfo         `json:"err,omitempty"`
	Data     map[string]any   `json:"data,omitempty"`
}

type Decision struct {
	Allow     bool   `json:"allow"`
	Scope     string `json:"scope,omitempty"` // once | session
	Reason    string `json:"reason,omitempty"`
	DecidedBy string `json:"decided_by,omitempty"`
}

type Capabilities struct {
	Streaming         bool     `json:"streaming"`
	Resume            bool     `json:"resume"`
	PermissionPrompts bool     `json:"permission_prompts"` // real approval protocol, not text sniffing
	PlanMode          bool     `json:"plan_mode"`
	ModelListing      bool     `json:"model_listing"`
	PermissionModes   []string `json:"permission_modes,omitempty"`
	// ModeRequiresRestart: the permission mode is fixed at process start, so a
	// mode change suspends an idle session and the next turn resumes with it.
	ModeRequiresRestart bool `json:"mode_requires_restart,omitempty"`
}

type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Tier string `json:"tier,omitempty"`
}

type StartOpts struct {
	ConvID    string
	Workspace string
	Mode      string
	Model     string
	Effort    string
	ResumeID  string // engine-side session id to resume ("" = fresh)
}

type UserInput struct {
	Text  string
	Media []string
	// Preamble is hub-provided context (handoff / working state) injected
	// before Text on the first turn after a start or engine switch.
	Preamble string
}

// Engine is a CLI backend (claude, codex, agy, ...).
type Engine interface {
	ID() string
	Capabilities() Capabilities
	Models(ctx context.Context) ([]Model, error)
	Start(ctx context.Context, o StartOpts) (Session, error)
}

// Session is one live conversation channel with an engine process.
type Session interface {
	EngineSessionID() string
	Send(ctx context.Context, in UserInput) error
	Events() <-chan Event // closed when the session ends
	Decide(approvalID string, d Decision) error
	SetMode(mode string) error
	Cancel() error // abort the current turn, keep the process
	Close() error  // end the process
}

// AuthStatus describes whether an engine can currently be used.
type AuthStatus struct {
	Installed bool   `json:"installed"`
	Known     bool   `json:"known"` // false when the CLI offers no way to check (try and see)
	LoggedIn  bool   `json:"logged_in"`
	Detail    string `json:"detail,omitempty"`
	LoginHint string `json:"login_hint,omitempty"`
}

// StatusProvider is optionally implemented by engines that can report auth state.
type StatusProvider interface {
	Status(ctx context.Context) AuthStatus
}

var (
	ErrBusy         = errors.New("session busy")
	ErrNoSuchEngine = errors.New("unknown engine")
	ErrNotLive      = errors.New("session not live")
	ErrBadState     = errors.New("invalid state transition")
	ErrNoApproval   = errors.New("no such pending approval")
	ErrNotLoggedIn  = errors.New("engine is not logged in")
)

// State of an engine binding.
type State string

const (
	StateCreated          State = "created"
	StateStarting         State = "starting"
	StateIdle             State = "idle"
	StateRunning          State = "running"
	StateAwaitingApproval State = "awaiting_approval"
	StateSuspended        State = "suspended"
	StateResuming         State = "resuming"
	StateFailed           State = "failed"
	StateStopped          State = "stopped"
)

var transitions = map[State][]State{
	StateCreated:          {StateStarting, StateStopped},
	StateStarting:         {StateIdle, StateRunning, StateFailed, StateStopped},
	StateIdle:             {StateRunning, StateSuspended, StateStopped, StateFailed},
	StateRunning:          {StateIdle, StateAwaitingApproval, StateFailed, StateStopped, StateSuspended},
	StateAwaitingApproval: {StateRunning, StateIdle, StateFailed, StateStopped, StateSuspended},
	StateSuspended:        {StateResuming, StateStarting, StateStopped},
	StateResuming:         {StateIdle, StateRunning, StateFailed, StateStopped},
	StateFailed:           {StateStarting, StateResuming, StateStopped, StateSuspended},
	StateStopped:          {StateStarting},
}

// CanTransition reports whether from -> to is a legal move (same state is a no-op and allowed).
func CanTransition(from, to State) bool {
	if from == to {
		return true
	}
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Live reports whether the state implies a running engine process.
func (s State) Live() bool {
	switch s {
	case StateStarting, StateIdle, StateRunning, StateAwaitingApproval, StateResuming:
		return true
	}
	return false
}

// Package core holds the engine-agnostic domain: events, engine/session
// contracts, capabilities and the session state machine. It imports only the
// standard library so adapters and the application layer both depend on it,
// never the other way round.
package core

import (
	"context"
	"errors"
	"strings"
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
	// EvShell is a command the user ran directly with "!cmd" (no model involved).
	EvShell EventType = "shell"
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
	// ContextTokens is how much of the model's context window the conversation occupies after this
	// turn (reported by the engine; 0 = unknown). ContextWindow is the model's window size.
	ContextTokens int `json:"context_tokens,omitempty"`
	ContextWindow int `json:"context_window,omitempty"`
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
	// ProfileDir selects an alternative config dir (a different account of the same engine); "" = default.
	ProfileDir string
	ConvID     string
	Workspace  string
	Mode       string
	Model      string
	Effort     string
	ResumeID   string // engine-side session id to resume ("" = fresh)
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

// Command is a slash command or skill an engine offers ("/name").
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ArgHint     string `json:"arg_hint,omitempty"`
	Kind        string `json:"kind,omitempty"` // command | skill
}

// CommandLister is implemented by engines that can enumerate their slash commands and skills.
type CommandLister interface {
	Commands(ctx context.Context) ([]Command, error)
}

// Summarizer is implemented by engines that can run a cheap, tool-less one-shot completion
// (used for rolling conversation summaries). Model reports which model it uses.
type Summarizer interface {
	Summarize(ctx context.Context, system, prompt string) (text string, usage Usage, err error)
	SummaryModel() string
	SummaryEngine() string // id of the owning engine (used to skip engines that are not signed in)
}

// CheapestModel picks the model most likely to be the cheapest from an engine's list, by well-known
// cost-tier words in the id/name. Returns "" when the list is empty.
func CheapestModel(ms []Model) string {
	if len(ms) == 0 {
		return ""
	}
	// most specific "small" markers first
	for _, kw := range []string{"nano", "lite", "mini", "haiku", "flash", "luna", "small"} {
		var best string
		for _, m := range ms {
			id := strings.ToLower(m.ID + " " + m.Name)
			if !strings.Contains(id, kw) {
				continue
			}
			// among those, prefer a low-effort variant
			if strings.Contains(id, "-low") || strings.Contains(id, "(low)") {
				return m.ID
			}
			if best == "" {
				best = m.ID
			}
		}
		if best != "" {
			return best
		}
	}
	return ms[0].ID
}

// QuotaWindow is one rate-limit window (for example "5 hours" or "Weekly").
type QuotaWindow struct {
	Label       string     `json:"label"`
	UsedPercent *float64   `json:"used_percent,omitempty"` // nil when the source does not say
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
	Disabled    bool       `json:"disabled,omitempty"`
}

// QuotaGroup groups windows that apply to the same set of models.
type QuotaGroup struct {
	Name    string        `json:"name"`
	Windows []QuotaWindow `json:"windows"`
}

// Quota is a provider-reported usage snapshot. Only what the CLI itself reports is included.
type Quota struct {
	Engine    string       `json:"engine"`
	Source    string       `json:"source"` // where the numbers come from
	Plan      string       `json:"plan,omitempty"`
	Groups    []QuotaGroup `json:"groups"`
	FetchedAt time.Time    `json:"fetched_at"`
}

// QuotaProvider is implemented by engines whose CLI can report real provider quota.
type QuotaProvider interface {
	Quota(ctx context.Context) (Quota, error)
}

type profileKey struct{}

// WithProfileDir makes engine-level calls (status, quota, models...) run against another account's config dir.
func WithProfileDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, profileKey{}, dir)
}

// ProfileDirFrom returns the config dir set by WithProfileDir ("" = default account).
func ProfileDirFrom(ctx context.Context) string {
	d, _ := ctx.Value(profileKey{}).(string)
	return d
}

// ProfileCapable engines can run against a separate config dir (one dir per account).
type ProfileCapable interface {
	ProfileEnv(dir string) []string
}

// LoginEnvProvider supplies extra environment for the login command (the account's config dir).
type LoginEnvProvider interface {
	LoginEnv() []string
}

// LoginProvider is implemented by engines that can sign in via a CLI command
// the hub drives (prints a link / code, may read a pasted code from stdin).
type LoginProvider interface {
	LoginCommand() (bin string, args []string)
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

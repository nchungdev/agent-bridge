package zalo

import (
	"context"
	"time"

	"github.com/nchungdev/agent-bridge/internal/core"
	"github.com/nchungdev/agent-bridge/internal/store"
)

// The interfaces below are what the bot needs from the rest of Agent Bridge. They are kept small and are
// defined here, where they are used, so the package depends on no concrete application type.

// TurnAgent runs conversation turns (manager.Manager satisfies it).
type TurnAgent interface {
	Subscribe(conv string) (<-chan core.Event, func())
	Send(ctx context.Context, conv, engineID string, in core.UserInput) error
	Decide(conv, approvalID string, d core.Decision) error
	Cancel(conv string) error
}

// ConvAdmin changes how a conversation is set up (manager.Manager satisfies it).
type ConvAdmin interface {
	SetConv(c store.ConvSettings) error
	SwitchEngine(conv, to string) error
	State(conv, engine string) core.State
}

// Agent is everything the bot asks of the agent runtime.
type Agent interface {
	TurnAgent
	ConvAdmin
}

// Settings persists the chat -> conversation mapping and reads conversation settings (store.Store satisfies it).
type Settings interface {
	GetSetting(key string) string
	SetSetting(key, value string) error
	GetConv(conv string) (*store.ConvSettings, error)
}

// Messenger delivers text and the typing indicator to a Zalo chat (*Client satisfies it).
type Messenger interface {
	SendText(ctx context.Context, chatID, text string) error
	Typing(ctx context.Context, chatID string) error
}

// ImageStore saves an image Zalo sent and returns the path of the saved file.
type ImageStore interface {
	Save(ctx context.Context, url string) (string, error)
}

// EngineInfo is an engine and the permission modes it supports (empty = no restriction).
type EngineInfo struct {
	ID    string
	Modes []string
}

// ConvInfo is one conversation the user can continue from Zalo.
type ConvInfo struct {
	ID        string
	Title     string
	Workspace string
	Updated   time.Time
	Archived  bool
}

// Deps are the application services the bot uses.
type Deps struct {
	Agent     Agent
	Settings  Settings
	Messenger Messenger
	Images    ImageStore
	// NewConv creates a conversation and returns its id.
	NewConv func(name, workspace string) (string, error)
	// PathOK reports whether a workspace folder may be used (server.PathAllowed).
	PathOK func(string) bool
	// Convs lists conversations.
	Convs func() ([]ConvInfo, error)
	// Engines lists the usable engines.
	Engines func() []EngineInfo
}

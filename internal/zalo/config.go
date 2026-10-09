// Package zalo connects a Zalo Bot (https://docs.zaloplatforms.com/docs/BOT) to Agent Bridge: messages the bot
// receives on its webhook become turns of an agent conversation, and the agent's answer is sent back to the chat.
package zalo

import (
	"os"
	"slices"
	"strings"
)

// Config comes from the environment (normally ~/.agent-bridge/agent-bridge.env).
type Config struct {
	Token     string          // ZALO_BOT_TOKEN: identifies the bot, part of every API path
	Secret    string          // ZALO_WEBHOOK_SECRET: must equal the secret_token given to setWebhook (8-256 chars)
	Allowed   map[string]bool // ZALO_ALLOWED_IDS: Zalo user ids that may drive agents
	Engine    string          // ZALO_ENGINE: engine id, default "claude"
	Mode      string          // ZALO_MODE: permission mode for Zalo conversations, default "plan" (read-only)
	Modes     []string        // ZALO_MODES: modes /mode may switch to (and the most /use leaves on a web conversation), default "plan,ask"
	UploadDir string          // where images sent to the bot are saved (set by the caller)
	// NASControl (ZALO_NAS_CONTROL=1) lets /nas:<service> start, stop, restart and update; without it /nas only reports.
	NASControl bool
	// NASUnits (ZALO_NAS_UNITS) are the systemd units /nas may list and control besides the Docker containers.
	NASUnits []string
	// NASProtected (ZALO_NAS_PROTECTED) are name fragments of services /nas may never stop, restart or update
	// from Zalo, because the bot depends on them (the tunnel, the gateway, Agent Bridge itself).
	NASProtected []string
	Workspace    string // ZALO_WORKSPACE: working folder, default the user's home
	APIBase      string // ZALO_API_BASE: override for tests, default the public Bot API
}

// DefaultAPIBase is the public Zalo Bot API.
const DefaultAPIBase = "https://bot-api.zaloplatforms.com"

// LoadConfig reads the environment. ok is false (the integration stays off) unless the token, a valid
// webhook secret and at least one allowed user id are set: with no allow-list nobody may drive an agent.
func LoadConfig() (cfg Config, ok bool) {
	cfg = Config{
		Token:     strings.TrimSpace(os.Getenv("ZALO_BOT_TOKEN")),
		Secret:    strings.TrimSpace(os.Getenv("ZALO_WEBHOOK_SECRET")),
		Allowed:   map[string]bool{},
		Engine:    envOr("ZALO_ENGINE", "claude"),
		Mode:      envOr("ZALO_MODE", "plan"),
		Workspace: strings.TrimSpace(os.Getenv("ZALO_WORKSPACE")),
		APIBase:   strings.TrimRight(envOr("ZALO_API_BASE", DefaultAPIBase), "/"),
	}
	cfg.NASControl = os.Getenv("ZALO_NAS_CONTROL") == "1"
	cfg.NASUnits = splitList(os.Getenv("ZALO_NAS_UNITS"))
	cfg.NASProtected = splitList(envOr("ZALO_NAS_PROTECTED", "cloudflared,wildcard-gateway,agent-bridge"))
	cfg.Modes = splitList(envOr("ZALO_MODES", "plan,ask"))
	if !slices.Contains(cfg.Modes, cfg.Mode) {
		cfg.Modes = append(cfg.Modes, cfg.Mode)
	}
	for _, id := range splitList(os.Getenv("ZALO_ALLOWED_IDS")) {
		cfg.Allowed[id] = true
	}
	ok = cfg.Token != "" && len(cfg.Secret) >= 8 && len(cfg.Secret) <= 256 && len(cfg.Allowed) > 0
	return cfg, ok
}

func envOr(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}

func splitList(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
}

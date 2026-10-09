// Package zalo connects a Zalo Bot (https://docs.zaloplatforms.com/docs/BOT) to Agent Bridge: messages the bot
// receives on its webhook become turns of an agent conversation, and the agent's answer is sent back to the chat.
package zalo

import (
	"os"
	"strings"
)

// Config comes from the environment (normally ~/.agent-bridge/agent-bridge.env).
type Config struct {
	Token     string          // ZALO_BOT_TOKEN: identifies the bot, part of every API path
	Secret    string          // ZALO_WEBHOOK_SECRET: must equal the secret_token given to setWebhook (8-256 chars)
	Allowed   map[string]bool // ZALO_ALLOWED_IDS: Zalo user ids that may drive agents
	Engine    string          // ZALO_ENGINE: engine id, default "claude"
	Mode      string          // ZALO_MODE: permission mode for Zalo conversations, default "plan" (read-only)
	Workspace string          // ZALO_WORKSPACE: working folder, default the user's home
	APIBase   string          // ZALO_API_BASE: override for tests, default the public Bot API
}

const defaultAPIBase = "https://bot-api.zaloplatforms.com"

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
		APIBase:   strings.TrimRight(envOr("ZALO_API_BASE", defaultAPIBase), "/"),
	}
	for _, id := range strings.FieldsFunc(os.Getenv("ZALO_ALLOWED_IDS"), func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
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

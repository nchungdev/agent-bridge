package agents

import (
	"fmt"
	"os/exec"
)

// Claude implements the Agent interface for Claude Code CLI.
type Claude struct {
	binaryPath string
}

func NewClaude(binaryPath string) *Claude {
	return &Claude{binaryPath: binaryPath}
}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) DisplayName() string { return "Claude Code" }

func (c *Claude) IsAvailable() bool {
	if c.binaryPath != "" {
		return true
	}
	// Check dynamically in PATH in case installed after service start
	return c.GetBinary() != ""
}

func (c *Claude) GetBinary() string {
	if c.binaryPath != "" {
		return c.binaryPath
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	return ""
}

func (c *Claude) SupportedModels() []ModelInfo {
	return []ModelInfo{
		{ID: "claude-sonnet-5-5", Name: "Claude Sonnet 5.5", Tier: "smart", Provider: "anthropic"},
		{ID: "claude-opus-5-5", Name: "Claude Opus 5.5", Tier: "thinking", Provider: "anthropic"},
		{ID: "claude-fable-5-1", Name: "Claude Fable 5.1", Tier: "thinking", Provider: "anthropic"},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Tier: "fast", Provider: "anthropic"},
		{ID: "claude-3-7-sonnet", Name: "Claude 3.7 Sonnet", Tier: "smart", Provider: "anthropic"},
	}
}

func (c *Claude) DefaultModel() string {
	return "claude-sonnet-5-5"
}

// BuildSpawnRequest prepares command-line arguments for spawning Claude Code CLI.
// Claude Code accepts -p for non-interactive prompt mode.
func (c *Claude) BuildSpawnRequest(sessionID, prompt, model, effort, workDir string) (*SpawnRequest, error) {
	bin := c.GetBinary()
	if bin == "" {
		return nil, fmt.Errorf("claude binary not found in system PATH. Install with: npm install -g @anthropic-ai/claude-code")
	}

	args := []string{}
	if prompt != "" {
		args = append(args, "-p", prompt)
	}

	return &SpawnRequest{
		BinaryPath: bin,
		Args:       args,
		WorkDir:    workDir,
		Env: []string{
			"TERM=xterm-256color",
		},
	}, nil
}

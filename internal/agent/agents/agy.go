package agents

import (
	"fmt"
	"strings"
)

// SpawnRequest holds all information needed to spawn an agent subprocess.
type SpawnRequest struct {
	BinaryPath string
	Args       []string
	Env        []string
	WorkDir    string
}

// AGY implements the Agent interface for Gemini CLI / Antigravity.
type AGY struct {
	binaryPath string
}

func NewAGY(binaryPath string) *AGY {
	return &AGY{binaryPath: binaryPath}
}

func (a *AGY) Name() string { return "agy" }

func (a *AGY) DisplayName() string { return "Gemini CLI (Antigravity)" }

func (a *AGY) IsAvailable() bool {
	return a.binaryPath != ""
}

func (a *AGY) SupportedModels() []ModelInfo {
	return []ModelInfo{
		{ID: "gemini-3.8-flash", Name: "Gemini 3.8 Flash", Tier: "fast", Provider: "google"},
		{ID: "gemini-3.5-flash", Name: "Gemini 3.5 Flash", Tier: "fast", Provider: "google"},
		{ID: "gemini-3.5-flash-lite", Name: "Gemini 3.5 Flash-Lite", Tier: "fast", Provider: "google"},
		{ID: "gemini-3.1-pro", Name: "Gemini 3.1 Pro", Tier: "smart", Provider: "google"},
	}
}

func (a *AGY) DefaultModel() string {
	return "gemini-3.8-flash"
}

// BuildSpawnRequest chuẩn bị cờ thực thi agy kèm theo effort level (low, medium, high) và conversation ID
func (a *AGY) BuildSpawnRequest(sessionID, prompt, model, effort, workDir string) (*SpawnRequest, error) {
	if !a.IsAvailable() {
		return nil, fmt.Errorf("agy binary not found at %s", a.binaryPath)
	}

	args := []string{}

	if prompt != "" {
		args = append(args, "-p", prompt)
	}

	// Nếu sessionID là một conversation ID có sẵn, truyền --conversation để tiếp tục phiên Antigravity
	if sessionID != "" && len(sessionID) >= 32 {
		args = append(args, "--conversation", sessionID)
	}

	if model != "" {
		args = append(args, "--model", model)
	}

	// Dynamic reasoning effort level: low, medium, high
	if effort == "" {
		effort = "medium"
	}
	args = append(args, "--effort", strings.ToLower(effort))

	// Stream NDJSON
	args = append(args, "--output-format", "stream-json", "--mode", "accept-edits")

	if workDir == "" {
		workDir = "/home/chungnh/AI Workspace"
	}

	return &SpawnRequest{
		BinaryPath: a.binaryPath,
		Args:       args,
		WorkDir:    workDir,
		Env: []string{
			"TERM=xterm-256color",
			"NO_COLOR=1",
		},
	}, nil
}

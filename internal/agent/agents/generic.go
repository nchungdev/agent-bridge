package agents

import (
	"fmt"
	"os/exec"
)

// Generic implements the AgentInterface for custom user-configured CLI tools.
type Generic struct {
	id          string
	displayName string
	binaryPath  string
	defaultMod  string
	envKey      string
	envVal      string
}

func NewGeneric(id, displayName, binaryPath, defaultMod, envKey, envVal string) *Generic {
	return &Generic{
		id:          id,
		displayName: displayName,
		binaryPath:  binaryPath,
		defaultMod:  defaultMod,
		envKey:      envKey,
		envVal:      envVal,
	}
}

func (g *Generic) Name() string { return g.id }

func (g *Generic) DisplayName() string {
	if g.displayName != "" {
		return g.displayName
	}
	return g.id
}

func (g *Generic) IsAvailable() bool {
	if g.binaryPath == "" {
		return false
	}
	_, err := exec.LookPath(g.binaryPath)
	return err == nil
}

func (g *Generic) SupportedModels() []ModelInfo {
	mod := g.DefaultModel()
	return []ModelInfo{
		{ID: mod, Name: mod, Tier: "smart", Provider: "custom"},
	}
}

func (g *Generic) DefaultModel() string {
	if g.defaultMod != "" {
		return g.defaultMod
	}
	return "default"
}

func (g *Generic) BuildSpawnRequest(sessionID, prompt, model, effort, workDir string) (*SpawnRequest, error) {
	if !g.IsAvailable() {
		return nil, fmt.Errorf("custom CLI binary '%s' not found in PATH", g.binaryPath)
	}

	args := []string{}
	if prompt != "" {
		args = append(args, "-p", prompt)
	}

	env := []string{
		"TERM=xterm-256color",
	}
	if g.envKey != "" && g.envVal != "" {
		env = append(env, fmt.Sprintf("%s=%s", g.envKey, g.envVal))
	}

	return &SpawnRequest{
		BinaryPath: g.binaryPath,
		Args:       args,
		WorkDir:    workDir,
		Env:        env,
	}, nil
}

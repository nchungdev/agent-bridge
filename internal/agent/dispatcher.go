package agent

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/nchungdev/agent-bridge/internal/agent/agents"
	"github.com/nchungdev/agent-bridge/internal/config"
)

// AgentInterface defines what each agent adapter must implement.
type AgentInterface interface {
	Name() string
	DisplayName() string
	IsAvailable() bool
	SupportedModels() []agents.ModelInfo
	DefaultModel() string
	BuildSpawnRequest(sessionID, prompt, model, effort, workDir string) (*agents.SpawnRequest, error)
}

// Dispatcher manages agent selection, spawning, and failover.
type Dispatcher struct {
	agents   map[string]AgentInterface
	fallback []string // Ordered list for failover
	mu       sync.RWMutex
	active   map[string]*Subprocess // session_id -> active subprocess
}

func NewDispatcher(cfg *config.Config) *Dispatcher {
	d := &Dispatcher{
		agents:   make(map[string]AgentInterface),
		active:   make(map[string]*Subprocess),
		fallback: []string{"agy", "claude", "codex"},
	}

	// Register available agents
	if agyCfg, ok := cfg.Agents["agy"]; ok {
		d.agents["agy"] = agents.NewAGY(agyCfg.BinaryPath)
	}
	if claudeCfg, ok := cfg.Agents["claude"]; ok {
		d.agents["claude"] = agents.NewClaude(claudeCfg.BinaryPath)
	}
	if codexCfg, ok := cfg.Agents["codex"]; ok {
		d.agents["codex"] = agents.NewCodex(codexCfg.BinaryPath)
	}

	return d
}

// RegisterAgent dynamically registers an AgentInterface
func (d *Dispatcher) RegisterAgent(a AgentInterface) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.agents[a.Name()] = a
}

// RegisterCustomAgent registers a custom user-defined agent
func (d *Dispatcher) RegisterCustomAgent(id, displayName, binaryPath, defaultMod, envKey, envVal string) {
	d.RegisterAgent(agents.NewGeneric(id, displayName, binaryPath, defaultMod, envKey, envVal))
}

// ListAgents returns information about all registered agents.
func (d *Dispatcher) ListAgents() []agents.AgentInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var list []agents.AgentInfo
	for _, a := range d.agents {
		list = append(list, agents.AgentInfo{
			ID:          a.Name(),
			DisplayName: a.DisplayName(),
			Available:   a.IsAvailable(),
			Models:      a.SupportedModels(),
			Default:     a.DefaultModel(),
		})
	}
	return list
}

// Dispatch spawns the requested agent and returns a stream of events.
func (d *Dispatcher) Dispatch(ctx context.Context, sessionID, agentName, model, effort, prompt, workDir string) (<-chan StreamEvent, error) {
	// Kill any active subprocess for this session
	d.KillSession(sessionID)

	ag, ok := d.agents[agentName]
	if !ok {
		return nil, fmt.Errorf("unknown agent: %s", agentName)
	}
	if !ag.IsAvailable() {
		ag = d.findFallback(agentName)
		if ag == nil {
			return nil, fmt.Errorf("agent %s not available and no fallback found", agentName)
		}
	}

	if model == "" {
		model = ag.DefaultModel()
	}

	proc, parser, err := d.spawnAgent(ctx, ag, sessionID, prompt, model, effort, workDir)
	if err != nil {
		return nil, fmt.Errorf("dispatch %s: %w", agentName, err)
	}

	d.mu.Lock()
	d.active[sessionID] = proc
	d.mu.Unlock()

	// Stream events with failover detection
	rawEvents := proc.Stream(parser)
	outputEvents := make(chan StreamEvent, 64)

	go func() {
		defer close(outputEvents)
		for evt := range rawEvents {
			if evt.Type == "error" && evt.Reason == "quota_exceeded" {
				log.Printf("[dispatcher] Quota exceeded for %s, attempting failover", agentName)
				fallbackAgent := d.findFallback(agentName)
				if fallbackAgent != nil {
					outputEvents <- StreamEvent{
						Type:   "agent_switch",
						From:   agentName,
						To:     fallbackAgent.Name(),
						Reason: "quota_exceeded",
					}
					proc.Kill()
					newProc, newParser, err := d.spawnAgent(ctx, fallbackAgent, sessionID, prompt, fallbackAgent.DefaultModel(), effort, workDir)
					if err == nil {
						d.mu.Lock()
						d.active[sessionID] = newProc
						d.mu.Unlock()
						for fEvt := range newProc.Stream(newParser) {
							outputEvents <- fEvt
						}
					}
				}
				continue
			}
			outputEvents <- evt
		}
	}()

	return outputEvents, nil
}

// spawnAgent converts a BuildSpawnRequest into an actual Subprocess.
func (d *Dispatcher) spawnAgent(ctx context.Context, ag AgentInterface, sessionID, prompt, model, effort, workDir string) (*Subprocess, *StreamParser, error) {
	req, err := ag.BuildSpawnRequest(sessionID, prompt, model, effort, workDir)
	if err != nil {
		return nil, nil, err
	}

	cfg := SpawnConfig{
		BinaryPath: req.BinaryPath,
		Args:       req.Args,
		Env:        req.Env,
		WorkDir:    req.WorkDir,
		Rows:       40,
		Cols:       120,
	}

	proc, err := Spawn(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	return proc, NewStreamParser(), nil
}

// SendInput sends text or approval response directly to the active agent process stdin
func (d *Dispatcher) SendInput(sessionID, input string) error {
	d.mu.RLock()
	proc, ok := d.active[sessionID]
	d.mu.RUnlock()

	if !ok || proc == nil {
		return fmt.Errorf("no active process for session %s", sessionID)
	}
	return proc.SendInput(input)
}

// KillSession terminates any active subprocess for the given session.
func (d *Dispatcher) KillSession(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if proc, ok := d.active[sessionID]; ok {
		proc.Kill()
		delete(d.active, sessionID)
	}
}

func (d *Dispatcher) findFallback(exclude string) AgentInterface {
	for _, name := range d.fallback {
		if name == exclude {
			continue
		}
		if a, ok := d.agents[name]; ok && a.IsAvailable() {
			return a
		}
	}
	return nil
}

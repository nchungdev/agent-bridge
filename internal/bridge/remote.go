package bridge

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// Remote access: the web and mobile apps of each agent only see sessions that were opened with the agent's remote
// mode on. The three CLIs do it differently:
//   - claude: per session, `claude --remote-control` (the session then shows up in claude.ai/code)
//   - agy:    per session, `agy --remote-control`, plus a background daemon (`agy remote-control start|status|stop`)
//   - codex:  one shared app-server daemon (`codex remote-control start|stop|pair`) that the web app connects to
// Turning these on registers this machine with the user's account, so nothing here runs unless asked.

// RemoteMode says how an agent's remote access is switched on.
type RemoteMode string

const (
	RemotePerSession RemoteMode = "per-session" // a flag on the CLI of each session
	RemoteDaemon     RemoteMode = "daemon"      // one background daemon for the machine
	RemoteBoth       RemoteMode = "both"
)

// RemoteModeOf returns "" for an unknown agent.
func RemoteModeOf(agent string) RemoteMode {
	switch BaseAgent(agent) {
	case "claude":
		return RemotePerSession
	case "agy":
		return RemoteBoth
	case "codex":
		return RemoteDaemon
	}
	return ""
}

// LaunchOpts describes how to start an agent CLI.
type LaunchOpts struct {
	ID     string // resume this session of the same agent
	Fresh  bool   // empty session, no handoff prompt
	Remote bool   // open it with the agent's remote access on (per-session agents)
	Name   string // label of the remote session (claude shows it in claude.ai/code)
	Title  string // the task's title: claude names its conversation after it (`--name`)
}

// cleanRemoteName keeps a label short and printable; "" falls back to a default.
func cleanRemoteName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > 60 {
		s = string(r[:60])
	}
	if s == "" {
		return "Agent Bridge"
	}
	return s
}

// BuildArgv is LaunchArgv/FreshArgv plus the remote flag and the conversation name (see naming.go). Nil for an
// unknown agent or a malformed session ID.
func BuildArgv(agent string, o LaunchOpts) []string {
	argv := buildArgv(agent, o)
	if flag := NameFlagArgs(agent, o.Title); argv != nil && flag != nil {
		return append([]string{argv[0]}, append(flag, argv[1:]...)...)
	}
	return argv
}

func buildArgv(agent string, o LaunchOpts) []string {
	var argv []string
	if o.Fresh {
		argv = FreshArgv(agent)
	} else {
		argv = LaunchArgv(agent, o.ID)
	}
	if argv == nil || !o.Remote {
		return argv
	}
	bin := argv[0]
	base := BaseAgent(agent)
	switch base {
	case "claude":
		// `--remote-control [name]` takes an optional value, so it must not be followed by the handoff prompt
		// unless it has a name of its own to swallow
		if n := len(argv); n > 1 && argv[n-1] == HandoffPrompt {
			out := append([]string{bin, "--remote-control", cleanRemoteName(firstNonEmpty(o.Name, o.Title))}, argv[1:]...)
			return out
		}
		return append(argv, "--remote-control")
	case "agy":
		return append([]string{bin, "--remote-control"}, argv[1:]...)
	}
	return argv // codex: remote access is the shared daemon, not a flag
}

// RemoteResult is the outcome of a remote-access command.
type RemoteResult struct {
	Agent  string `json:"agent"`
	Mode   string `json:"mode"`
	Output string `json:"output"`
	OK     bool   `json:"ok"`
}

// remoteArgv maps (agent, action) to the CLI command that does it; nil when the agent has no such action.
func remoteArgv(agent, action string) []string {
	base := BaseAgent(agent)
	bin := agent
	switch base + " " + action {
	case "agy status":
		return []string{bin, "remote-control", "status"}
	case "agy enable":
		return []string{bin, "remote-control", "start"}
	case "agy disable":
		return []string{bin, "remote-control", "stop"}
	case "codex enable":
		return []string{bin, "remote-control", "start"}
	case "codex disable":
		return []string{bin, "remote-control", "stop"}
	case "codex pair":
		return []string{bin, "remote-control", "pair"}
	}
	return nil
}

// RemoteAction runs a remote-access command through a login shell (the same PATH the terminals get).
func RemoteAction(ctx context.Context, shell, agent, action string) (*RemoteResult, error) {
	argv := remoteArgv(agent, action)
	if argv == nil {
		return nil, fmt.Errorf("%s has no %q remote action", agent, action)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, shell, "-lc", ShellJoin(argv))
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	res := &RemoteResult{Agent: agent, Mode: string(RemoteModeOf(agent)), Output: strings.TrimSpace(out.String()), OK: err == nil}
	if ctx.Err() == context.DeadlineExceeded {
		res.Output += "\n(timed out after 30s)"
	}
	return res, nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

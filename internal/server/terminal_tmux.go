package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Terminals live in tmux when it is installed: tmux keeps the shell or agent CLI running outside this
// process, so restarting the server (or the browser tab going away) no longer kills them. The server
// only attaches a PTY to `tmux attach`. A dedicated socket keeps them apart from the user's own tmux;
// from a shell: tmux -L agent-bridge attach -t <id>. Without tmux the in-process PTY registry is used.
const tmuxSocket = "agent-bridge"

const tmuxConf = `set -g status off
set -g mouse on
set -g history-limit 50000
set -g escape-time 0
set -g window-size latest
set -g default-terminal "screen-256color"
set -ga terminal-overrides ",xterm*:Tc"
`

var (
	tmuxOnce sync.Once
	tmuxPath string
)

// tmuxBin returns the tmux binary, or "" when tmux is unavailable or disabled (AGENT_BRIDGE_TMUX=0).
func tmuxBin() string {
	if os.Getenv("AGENT_BRIDGE_TMUX") == "0" {
		return ""
	}
	tmuxOnce.Do(func() {
		if p, err := exec.LookPath("tmux"); err == nil {
			tmuxPath = p
			return
		}
		if home, err := os.UserHomeDir(); err == nil {
			p := filepath.Join(home, ".local", "bin", "tmux")
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				tmuxPath = p
			}
		}
	})
	return tmuxPath
}

func tmuxConfPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".agent-bridge")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "tmux.conf")
	if b, err := os.ReadFile(p); err != nil || string(b) != tmuxConf {
		if err := os.WriteFile(p, []byte(tmuxConf), 0o600); err != nil {
			return "", err
		}
	}
	return p, nil
}

func tmuxCmd(args ...string) (*exec.Cmd, error) {
	conf, err := tmuxConfPath()
	if err != nil {
		return nil, err
	}
	full := append([]string{"-L", tmuxSocket, "-f", conf}, args...)
	cmd := exec.Command(tmuxBin(), full...)
	cmd.Env = withoutEnv(os.Environ(), "TMUX")
	return cmd, nil
}

func withoutEnv(env []string, key string) []string {
	out := env[:0:0]
	for _, e := range env {
		if !strings.HasPrefix(e, key+"=") {
			out = append(out, e)
		}
	}
	return out
}

func tmuxRun(args ...string) ([]byte, error) {
	cmd, err := tmuxCmd(args...)
	if err != nil {
		return nil, err
	}
	return cmd.CombinedOutput()
}

// ensureTmuxSession starts the tmux session id running argv in dir unless it already exists.
func ensureTmuxSession(id, dir, agent, resume string, argv []string) error {
	if _, err := tmuxRun("has-session", "-t", "="+id); err == nil {
		return nil
	}
	args := append([]string{"new-session", "-d", "-s", id, "-c", dir}, argv...)
	args = append(args, ";", "set-option", "-t", id, "@agent", agent, ";", "set-option", "-t", id, "@resume", resume)
	if out, err := tmuxRun(args...); err != nil {
		if _, herr := tmuxRun("has-session", "-t", "="+id); herr == nil {
			return nil // another attach created it first
		}
		return fmt.Errorf("tmux new-session: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func tmuxAttachCmd(id string) (*exec.Cmd, error) {
	return tmuxCmd("attach-session", "-t", "="+id)
}

type termInfo struct {
	ID     string `json:"id"`
	Dir    string `json:"dir"`
	Agent  string `json:"agent,omitempty"`
	Resume string `json:"resume,omitempty"`
}

func tmuxList() []termInfo {
	out, err := tmuxRun("list-sessions", "-F", "#{session_name}\t#{session_path}\t#{@agent}\t#{@resume}")
	if err != nil {
		return nil // no server running means no sessions
	}
	var list []termInfo
	// not TrimSpace: empty trailing fields are tabs
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 4 || !shellIDRe.MatchString(f[0]) {
			continue
		}
		list = append(list, termInfo{ID: f[0], Dir: f[1], Agent: f[2], Resume: f[3]})
	}
	return list
}

func tmuxKill(id string) { _, _ = tmuxRun("kill-session", "-t", "="+id) }

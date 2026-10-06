package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Terminals live in tmux when it is installed: tmux keeps the shell or agent CLI running outside this
// process, so restarting the server (or the browser tab going away) no longer kills them. The server
// only attaches a PTY to `tmux attach`. A dedicated socket keeps them apart from the user's own tmux;
// from a shell: tmux -S ~/.agent-bridge/tmux.sock attach -t <id>. Without tmux the in-process PTY registry is used.

// tmuxSocketArgs selects the tmux server. The socket is a file in the data directory, not a name under /tmp:
// a service with PrivateTmp (or a restarted one) would otherwise not find the tmux server again, and a file
// under the data dir is also reachable from any shell. AGENT_BRIDGE_TMUX_SOCKET overrides it with a -L name,
// which the tests use to stay clear of the real terminals.
func tmuxSocketArgs() []string {
	if v := os.Getenv("AGENT_BRIDGE_TMUX_SOCKET"); v != "" {
		return []string{"-L", v}
	}
	dir := os.Getenv("AGENT_BRIDGE_DATA_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".agent-bridge")
	}
	_ = os.MkdirAll(dir, 0o700)
	return []string{"-S", filepath.Join(dir, "tmux.sock")}
}

const tmuxConf = `set -g status off
set -g mouse on
set -g history-limit 50000
set -g escape-time 0
set -g window-size latest
setw -g aggressive-resize on
set -g default-terminal "xterm-256color"
set -ga terminal-overrides ",xterm*:Tc:Sync"
set -g set-clipboard on

# Drag selection puts text into tmux buffer without clearing highlight
bind -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-no-clear
bind -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-selection-no-clear

# Wheel scrolling:
# In root mode (normal shell), WheelUpPane enters copy-mode with -e and scrolls up 3 lines
bind -T root WheelUpPane if-shell -F "#{||:#{pane_in_mode},#{mouse_any_flag}}" "send-keys -M" "copy-mode -e ; send-keys -X -N 3 scroll-up"

# In copy-mode, Wheel scrolls by 3 lines for smooth responsive scrolling (copy-mode -e auto-exits at bottom)
bind -T copy-mode WheelUpPane select-pane \; send-keys -X -N 3 scroll-up
bind -T copy-mode WheelDownPane select-pane \; send-keys -X -N 3 scroll-down
bind -T copy-mode-vi WheelUpPane select-pane \; send-keys -X -N 3 scroll-up
bind -T copy-mode-vi WheelDownPane select-pane \; send-keys -X -N 3 scroll-down

# Touch single-line scrolling (shift+wheel):
bind -n S-WheelUpPane if -F '#{pane_in_mode}' 'send-keys -X scroll-up' 'copy-mode -e ; send-keys -X scroll-up'
bind -n S-WheelDownPane if -F '#{pane_in_mode}' 'send-keys -X scroll-down' ''
bind -T copy-mode S-WheelUpPane send-keys -X scroll-up
bind -T copy-mode S-WheelDownPane send-keys -X scroll-down
bind -T copy-mode-vi S-WheelUpPane send-keys -X scroll-up
bind -T copy-mode-vi S-WheelDownPane send-keys -X scroll-down
`

var (
	tmuxOnce sync.Once
	confOnce sync.Once
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
	// a tmux server that outlives us only reads its config when it starts: apply the current one once per process
	// (it is fine when no server is running yet)
	confOnce.Do(func() { _ = exec.Command(tmuxBin(), append(tmuxSocketArgs(), "source-file", p)...).Run() })
	return p, nil
}

func tmuxCmd(args ...string) (*exec.Cmd, error) {
	conf, err := tmuxConfPath()
	if err != nil {
		return nil, err
	}
	full := append(append(tmuxSocketArgs(), "-f", conf), args...)
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

// tmuxResizeWindow explicitly resizes the tmux window for this session.
func tmuxResizeWindow(id string, cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return nil
	}
	_, err := tmuxRun("resize-window", "-t", "="+id, "-x", strconv.Itoa(int(cols)), "-y", strconv.Itoa(int(rows)))
	return err
}

// ensureTmuxSession starts the tmux session id running argv in dir unless it already exists.
func ensureTmuxSession(id, dir, agent, resume string, argv []string, cols, rows uint16) error {
	if _, err := tmuxRun("has-session", "-t", "="+id); err == nil {
		if cols > 0 && rows > 0 {
			_ = tmuxResizeWindow(id, cols, rows)
		}
		return nil
	}
	args := []string{"new-session", "-d", "-s", id, "-c", dir}
	if cols > 0 && rows > 0 {
		args = append(args, "-x", strconv.Itoa(int(cols)), "-y", strconv.Itoa(int(rows)))
	}
	args = append(args, argv...)
	args = append(args, ";", "set-option", "-t", id, "@agent", agent, ";", "set-option", "-t", id, "@resume", resume)
	if out, err := tmuxRun(args...); err != nil {
		if _, herr := tmuxRun("has-session", "-t", "="+id); herr == nil {
			return nil // another attach created it first
		}
		return fmt.Errorf("tmux new-session: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// typeWhenReady types text into a tmux session once its screen has settled (the CLI is waiting at its prompt).
// It gives up after 30s, and never types over a dialog (trust folder, update): that would answer it.
func typeWhenReady(id, text string) {
	prev := ""
	for i := 0; i < 60; i++ {
		time.Sleep(500 * time.Millisecond)
		out, err := tmuxRun("capture-pane", "-p", "-t", "="+id)
		if err != nil {
			return // the session is gone
		}
		screen := string(out)
		low := strings.ToLower(screen)
		blocked := strings.Contains(low, "do you trust") || strings.Contains(low, "update available") || strings.Contains(low, "update now")
		if strings.TrimSpace(screen) != "" && screen == prev && !blocked {
			_ = tmuxSendKeys(id, text)
			return
		}
		prev = screen
	}
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

func tmuxSendKeys(id, text string) error {
	if strings.HasSuffix(text, "\n") {
		trimmed := strings.TrimSuffix(text, "\n")
		if trimmed != "" {
			_, _ = tmuxRun("send-keys", "-t", "="+id, "-l", trimmed)
		}
		_, err := tmuxRun("send-keys", "-t", "="+id, "Enter")
		return err
	}
	_, err := tmuxRun("send-keys", "-t", "="+id, "-l", text)
	return err
}

// tmuxBuffer returns the newest tmux paste buffer: what a mouse selection inside the terminal copied.
func tmuxBuffer() (string, error) {
	out, err := tmuxRun("show-buffer")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

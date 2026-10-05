package bridge

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ContextUsage is how full a session's context window was after the engine's last reply,
// exactly as the engine's own transcript recorded it (nothing is estimated).
type ContextUsage struct {
	Agent     string `json:"agent"`
	Model     string `json:"model,omitempty"`
	Tokens    int    `json:"tokens"`
	Window    int    `json:"window"`
	Supported bool   `json:"supported"`
}

// tailLines returns the last lines of a file (reads at most maxBytes from the end).
func tailLines(path string, maxBytes int64) [][]byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	start := info.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	lines := bytes.Split(data, []byte("\n"))
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // the first line is cut off mid-way
	}
	return lines
}

// GetContextUsage reads the latest context-window usage for a native session.
func (m *Manager) GetContextUsage(agent, id, workspace string) ContextUsage {
	out := ContextUsage{Agent: agent}
	if id == "" || !nativeIDRe.MatchString(id) {
		return out
	}
	switch agent {
	case "claude":
		path := filepath.Join(claudeProjectDir(workspace), id+".jsonl")
		if _, err := os.Stat(path); err != nil {
			// Claude files a session under the folder it was started in, which is not always the folder the
			// caller has in hand (a tab in a sub-folder, a session that moved): the session ID is unique anyway
			if found, _ := filepath.Glob(filepath.Join(homeDir(), ".claude", "projects", "*", id+".jsonl")); len(found) > 0 {
				path = found[0]
			}
		}
		out.Tokens, out.Window, out.Model = claudeContextUsage(path)
	case "codex":
		var path string
		_ = filepath.Walk(filepath.Join(homeDir(), ".codex", "sessions"), func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(p, ".jsonl") && strings.Contains(p, id) {
				path = p
			}
			return nil
		})
		if path != "" {
			out.Tokens, out.Window = codexContextUsage(path)
		}
	}
	out.Supported = out.Window > 0
	return out
}

func claudeContextUsage(path string) (tokens, window int, model string) {
	lines := tailLines(path, 4<<20)
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"usage"`)) {
			continue
		}
		var line struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Model string `json:"model"`
				Usage struct {
					Input         int `json:"input_tokens"`
					CacheCreation int `json:"cache_creation_input_tokens"`
					CacheRead     int `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &line) != nil || line.Type != "assistant" || line.IsSidechain {
			continue
		}
		u := line.Message.Usage
		tokens = u.Input + u.CacheCreation + u.CacheRead
		if tokens == 0 || strings.HasPrefix(line.Message.Model, "<") {
			continue // synthetic message
		}
		// the transcript does not record the window: 200k, or 1M once a session has outgrown 200k
		window = 200000
		if tokens > window || strings.Contains(line.Message.Model, "[1m]") {
			window = 1000000
		}
		return tokens, window, line.Message.Model
	}
	return 0, 0, ""
}

func codexContextUsage(path string) (tokens, window int) {
	lines := tailLines(path, 4<<20)
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"token_count"`)) {
			continue
		}
		var line struct {
			Payload struct {
				Type string `json:"type"`
				Info *struct {
					Last struct {
						Total int `json:"total_tokens"`
					} `json:"last_token_usage"`
					Window int `json:"model_context_window"`
				} `json:"info"`
			} `json:"payload"`
		}
		if json.Unmarshal(lines[i], &line) != nil || line.Payload.Type != "token_count" || line.Payload.Info == nil {
			continue
		}
		return line.Payload.Info.Last.Total, line.Payload.Info.Window
	}
	return 0, 0
}

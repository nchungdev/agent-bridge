package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

// StreamEvent represents a parsed event from CLI output.
type StreamEvent struct {
	Type      string `json:"type"`                 // "token", "diff", "tool_request", "tool_result", "agent_switch", "done", "task_finished", "session_created", "error"
	SessionID string `json:"session_id,omitempty"` // For session_created
	Content   string `json:"content,omitempty"`    // Text content for token events
	File      string `json:"file,omitempty"`       // File path for diff events
	Patch     string `json:"patch,omitempty"`      // Diff patch content
	Tool      string `json:"tool,omitempty"`       // Tool name for tool_request
	Command   string `json:"command,omitempty"`    // Command for tool_request
	Output    string `json:"output,omitempty"`     // Output for tool_result
	From      string `json:"from,omitempty"`       // For agent_switch
	To        string `json:"to,omitempty"`         // For agent_switch
	Reason    string `json:"reason,omitempty"`     // For agent_switch
	Message   string `json:"message,omitempty"`    // For error events
	Usage     *Usage `json:"usage,omitempty"`      // For done events

	RequiresApproval bool `json:"requires_approval,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Subprocess manages a CLI agent running via standard OS Pipes.
type Subprocess struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stdin  io.WriteCloser
	cancel context.CancelFunc
	mu     sync.Mutex
	done   chan struct{}
}

// SpawnConfig holds configuration for spawning a subprocess.
type SpawnConfig struct {
	BinaryPath string
	Args       []string
	Env        []string
	WorkDir    string
	Rows       uint16
	Cols       uint16
}

// Spawn creates a new subprocess using reliable standard OS Pipes.
// Avoids PTY /dev/ptmx EIO (input/output error) on process exit.
func Spawn(ctx context.Context, cfg SpawnConfig) (*Subprocess, error) {
	ctx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(ctx, cfg.BinaryPath, cfg.Args...)

	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}

	cmd.Env = append(os.Environ(), cfg.Env...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("pipe stdout: %w", err)
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("pipe stdin: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("spawn process: %w", err)
	}

	return &Subprocess{
		cmd:    cmd,
		stdout: stdoutPipe,
		stdin:  stdinPipe,
		cancel: cancel,
		done:   make(chan struct{}),
	}, nil
}

// Stream reads output line-by-line using bufio.Scanner to push tokens with zero latency.
func (s *Subprocess) Stream(parser *StreamParser) <-chan StreamEvent {
	events := make(chan StreamEvent, 64)

	go func() {
		defer close(events)
		defer close(s.done)

		scanner := bufio.NewScanner(s.stdout)
		// Cho phép parse dòng dài (đặc biệt khi JSON chứa văn bản lớn)
		buf := make([]byte, 64*1024)
		scanner.Buffer(buf, 10*1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			parsed := parser.Parse(line)
			for _, evt := range parsed {
				events <- evt
			}
		}

		if err := scanner.Err(); err != nil && err != io.EOF {
			// Bỏ qua lỗi EIO nếu process đóng stream tự nhiên
			errStr := err.Error()
			if !strings.Contains(errStr, "input/output error") {
				events <- StreamEvent{
					Type:    "error",
					Message: fmt.Sprintf("stream scan error: %v", err),
				}
			}
		}

		// Wait for process completion
		s.cmd.Wait()
		events <- StreamEvent{Type: "task_finished"}
		events <- StreamEvent{Type: "done"}
	}()

	return events
}

// SendInput writes to stdin pipe.
func (s *Subprocess) SendInput(input string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stdin != nil {
		_, err := io.WriteString(s.stdin, input)
		return err
	}
	return nil
}

// Kill terminates the subprocess gracefully.
func (s *Subprocess) Kill() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			s.cmd.Process.Kill()
		}
	}
	s.cancel()

	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.stdout != nil {
		s.stdout.Close()
	}
	return nil
}

func (s *Subprocess) Wait() {
	<-s.done
}

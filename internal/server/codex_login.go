package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// codexDeviceLogin owns the one account-wide device-auth process. The CLI
// remains the sole credential store; this manager retains only short-lived
// device-login instructions for the authenticated web session.
type codexDeviceLogin struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	output   bytes.Buffer
	finished bool
	exitErr  string
	changed  chan struct{}
}

type codexDeviceLoginState struct {
	Running  bool   `json:"running"`
	Finished bool   `json:"finished"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	LoggedIn bool   `json:"logged_in"`
}

var activeCodexDeviceLogin = &codexDeviceLogin{changed: make(chan struct{}, 1)}

func (l *codexDeviceLogin) appendOutput(text string) {
	if text == "" {
		return
	}
	l.mu.Lock()
	if l.output.Len()+len(text) > 16*1024 {
		current := l.output.String()
		l.output.Reset()
		l.output.WriteString(current[len(current)/2:])
	}
	l.output.WriteString(text)
	changed := l.changed
	l.mu.Unlock()
	select {
	case changed <- struct{}{}:
	default:
	}
}

func (l *codexDeviceLogin) readOutput(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	for scanner.Scan() {
		l.appendOutput(scanner.Text() + "\n")
	}
}

func (l *codexDeviceLogin) snapshot() codexDeviceLoginState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return codexDeviceLoginState{
		Running:  l.cmd != nil && !l.finished,
		Finished: l.finished,
		Output:   l.output.String(),
		Error:    l.exitErr,
	}
}

func (l *codexDeviceLogin) start(binary string) (codexDeviceLoginState, error) {
	l.mu.Lock()
	if l.cmd != nil && !l.finished {
		state := codexDeviceLoginState{Running: true, Output: l.output.String()}
		l.mu.Unlock()
		return state, nil
	}

	cmd := exec.Command(binary, "login", "--device-auth")
	cmd.Env = os.Environ()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		l.mu.Unlock()
		return codexDeviceLoginState{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		l.mu.Unlock()
		return codexDeviceLoginState{}, err
	}
	if err := cmd.Start(); err != nil {
		l.mu.Unlock()
		return codexDeviceLoginState{}, err
	}

	l.cmd = cmd
	l.output.Reset()
	l.finished = false
	l.exitErr = ""
	l.changed = make(chan struct{}, 1)
	changed := l.changed
	l.mu.Unlock()

	go l.readOutput(stdout)
	go l.readOutput(stderr)
	go func() {
		err := cmd.Wait()
		l.mu.Lock()
		l.finished = true
		if err != nil {
			l.exitErr = err.Error()
		}
		l.mu.Unlock()
		select {
		case changed <- struct{}{}:
		default:
		}
	}()

	// Give the CLI a brief window to print its one-time verification link/code.
	select {
	case <-changed:
	case <-time.After(1500 * time.Millisecond):
	}
	return l.snapshot(), nil
}

func codexLoggedIn(binary string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "login", "status").CombinedOutput()
	return err == nil, strings.TrimSpace(string(out))
}

func handleCodexDeviceLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			httpError(w, err, http.StatusBadRequest)
			return
		}

		binary, err := exec.LookPath("codex")
		if err != nil {
			jsonResponse(w, map[string]any{
				"success": false,
				"error":   "Codex CLI is not installed on the NAS yet. Install it first, then start device login.",
			})
			return
		}

		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "", "start":
			state, err := activeCodexDeviceLogin.start(binary)
			if err != nil {
				jsonResponse(w, map[string]any{"success": false, "error": err.Error()})
				return
			}
			state.LoggedIn, _ = codexLoggedIn(binary)
			jsonResponse(w, map[string]any{"success": true, "state": state})
		case "status":
			state := activeCodexDeviceLogin.snapshot()
			loggedIn, status := codexLoggedIn(binary)
			state.LoggedIn = loggedIn
			if state.LoggedIn {
				state.Error = ""
			}
			jsonResponse(w, map[string]any{"success": true, "state": state, "status": status})
		default:
			jsonResponse(w, map[string]any{"success": false, "error": "Unsupported Codex login action"})
		}
	}
}

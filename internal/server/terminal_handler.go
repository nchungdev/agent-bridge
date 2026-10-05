package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/nchungdev/agent-hub/internal/bridge"
)

// TerminalSession manages an interactive PTY session connected via WebSocket
type TerminalSession struct {
	ptmx *os.File
	cmd  *exec.Cmd
	once sync.Once
}

func handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[terminal] upgrade error: %v", err)
		return
	}
	defer conn.Close()

	workDir := r.URL.Query().Get("dir")
	if workDir == "" {
		workDir, _ = os.UserHomeDir()
	}
	if !PathAllowed(workDir) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31mworking directory is outside the allowed workspace roots\x1b[0m\r\n"))
		return
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmd := exec.Command(shell)
	// ?agent=claude|codex|agy[&resume=<native id>] runs that agent's CLI in the PTY instead of a bare
	// shell (a login shell, so PATH from the user's profile applies); the shell stays open after it exits.
	if agent := r.URL.Query().Get("agent"); agent != "" {
		argv := bridge.LaunchArgv(agent, r.URL.Query().Get("resume"))
		if argv == nil {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31munknown agent or invalid session id\x1b[0m\r\n"))
			return
		}
		cmd = exec.Command(shell, "-lc", bridge.ShellJoin(argv)+"; exec "+bridge.ShellJoin([]string{shell})+" -l")
	}
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("[terminal] pty start error: %v", err)
		return
	}
	defer func() {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	var writeMu sync.Mutex

	// Read from PTY and send to WebSocket. Binary frames: a read can end in the middle of a multi-byte
	// UTF-8 character, which would make an invalid text frame (browsers drop the connection on those).
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				writeMu.Lock()
				_ = conn.WriteMessage(websocket.BinaryMessage, buf[:n])
				writeMu.Unlock()
			}
			if err != nil {
				break
			}
		}
		_ = cmd.Wait() // reap the shell
		writeMu.Lock()
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shell exited"), time.Now().Add(time.Second))
		writeMu.Unlock()
	}()

	// Read from WebSocket and write to PTY
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if messageType == 1 { // Text / JSON or raw text
			// Check if message is a resize message {"type":"resize","cols":80,"rows":24}
			var msg struct {
				Type string `json:"type"`
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
				Data string `json:"data"`
			}
			if err := json.Unmarshal(p, &msg); err == nil && msg.Type == "resize" {
				_ = pty.Setsize(ptmx, &pty.Winsize{
					Rows: msg.Rows,
					Cols: msg.Cols,
				})
				continue
			} else if err == nil && msg.Type == "input" {
				_, _ = ptmx.Write([]byte(msg.Data))
				continue
			}

			// Raw input fallback
			_, _ = ptmx.Write(p)
		} else if messageType == 2 { // BinaryMessage
			_, _ = ptmx.Write(p)
		}
	}
}

// Simple REST endpoint to execute a quick command and get output
func handleTerminalExec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Command string `json:"command"`
		Dir     string `json:"dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	if req.Command == "" {
		httpError(w, io.ErrUnexpectedEOF, http.StatusBadRequest)
		return
	}

	workDir := req.Dir
	if workDir == "" {
		workDir = "/home/chungnh/AI Workspace"
	}

	cmd := exec.Command("bash", "-c", req.Command)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	jsonResponse(w, map[string]any{
		"output":    string(out),
		"exit_code": exitCode,
	})
}

package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"sync"
	"syscall"

	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/nchungdev/agent-hub/internal/bridge"
)

const scrollbackMax = 512 * 1024

// ptySession is a PTY plus its scrollback. Shells opened with ?id= are persistent: they outlive the
// WebSocket, so reloading the page (or opening it from another device) re-attaches to the same
// terminal with its screen content. Agent CLI tabs are not persistent: closing the tab kills them.
type ptySession struct {
	id         string
	dir        string
	persistent bool
	ptmx       *os.File
	cmd        *exec.Cmd

	mu    sync.Mutex
	buf   []byte
	conns map[*websocket.Conn]struct{}
	dead  bool
}

var (
	ptyMu       sync.Mutex
	ptyRegistry = map[string]*ptySession{}
)

func (ps *ptySession) kill() {
	_ = ps.ptmx.Close()
	if ps.cmd.Process != nil {
		// the PTY child leads its own session/process group: kill the whole group so an agent CLI
		// started by the shell dies with the tab instead of lingering
		_ = syscall.Kill(-ps.cmd.Process.Pid, syscall.SIGKILL)
		_ = ps.cmd.Process.Kill()
	}
}

// pump copies PTY output to scrollback and every attached connection until the process exits.
func (ps *ptySession) pump() {
	buf := make([]byte, 8192)
	for {
		n, err := ps.ptmx.Read(buf)
		if n > 0 {
			ps.mu.Lock()
			ps.buf = append(ps.buf, buf[:n]...)
			if over := len(ps.buf) - scrollbackMax; over > 0 {
				ps.buf = append([]byte(nil), ps.buf[over:]...)
			}
			// binary frames: a read can end inside a multi-byte UTF-8 character
			for c := range ps.conns {
				_ = c.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			ps.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	_ = ps.cmd.Wait() // reap the shell
	ps.mu.Lock()
	ps.dead = true
	for c := range ps.conns {
		_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shell exited"), time.Now().Add(time.Second))
	}
	ps.mu.Unlock()
	if ps.persistent {
		ptyMu.Lock()
		if ptyRegistry[ps.id] == ps {
			delete(ptyRegistry, ps.id)
		}
		ptyMu.Unlock()
	}
}

func startPTY(cmd *exec.Cmd, dir, id string, persistent bool) (*ptySession, error) {
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	ps := &ptySession{id: id, dir: dir, persistent: persistent, ptmx: ptmx, cmd: cmd, conns: map[*websocket.Conn]struct{}{}}
	go ps.pump()
	return ps, nil
}

func handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[terminal] upgrade error: %v", err)
		return
	}
	defer conn.Close()

	q := r.URL.Query()
	workDir := q.Get("dir")
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

	id := q.Get("id")
	agent := q.Get("agent")
	if agent != "" {
		id = "" // agent tabs are never persistent
	} else if id != "" && !shellIDRe.MatchString(id) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31minvalid terminal id\x1b[0m\r\n"))
		return
	}

	var ps *ptySession
	if id != "" {
		ptyMu.Lock()
		ps = ptyRegistry[id]
		ptyMu.Unlock()
	}
	if ps == nil {
		cmd := exec.Command(shell)
		// ?agent=claude|codex|agy[&resume=<native id>] runs that agent's CLI in the PTY instead of a bare
		// shell (a login shell, so PATH from the user's profile applies); the shell stays open after it exits.
		if agent != "" {
			argv := bridge.LaunchArgv(agent, q.Get("resume"))
			if argv == nil {
				_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31munknown agent or invalid session id\x1b[0m\r\n"))
				return
			}
			cmd = exec.Command(shell, "-lc", bridge.ShellJoin(argv)+"; exec "+bridge.ShellJoin([]string{shell})+" -l")
		}
		ps, err = startPTY(cmd, workDir, id, id != "")
		if err != nil {
			log.Printf("[terminal] pty start error: %v", err)
			return
		}
		if id != "" {
			ptyMu.Lock()
			ptyRegistry[id] = ps
			ptyMu.Unlock()
		}
	}

	// replay the screen so far, then join the live stream
	ps.mu.Lock()
	if len(ps.buf) > 0 {
		_ = conn.WriteMessage(websocket.BinaryMessage, ps.buf)
	}
	dead := ps.dead
	if !dead {
		ps.conns[conn] = struct{}{}
	}
	ps.mu.Unlock()
	if dead {
		return
	}
	defer func() {
		ps.mu.Lock()
		delete(ps.conns, conn)
		ps.mu.Unlock()
		if !ps.persistent {
			ps.kill()
		}
	}()

	// Read from WebSocket and write to PTY
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			break
		}
		ptmx := ps.ptmx

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

var shellIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// handleTerminalList lists the persistent shells that are still running.
func handleTerminalList(w http.ResponseWriter, r *http.Request) {
	type item struct {
		ID  string `json:"id"`
		Dir string `json:"dir"`
	}
	out := []item{}
	ptyMu.Lock()
	for _, ps := range ptyRegistry {
		out = append(out, item{ps.id, ps.dir})
	}
	ptyMu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// handleTerminalKill ends a persistent shell.
func handleTerminalKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ptyMu.Lock()
	ps := ptyRegistry[id]
	delete(ptyRegistry, id)
	ptyMu.Unlock()
	if ps != nil {
		ps.kill()
	}
	w.WriteHeader(http.StatusNoContent)
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

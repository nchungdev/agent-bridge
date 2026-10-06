package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/nchungdev/agent-bridge/internal/bridge"
)

const scrollbackMax = 512 * 1024

// ptySession is a PTY plus its scrollback. Sessions opened with ?id= (shells and agent CLIs) are
// persistent: they outlive the WebSocket, so reloading the page (or opening it from another device)
// re-attaches to the same terminal with its screen content. They end only when the tab is closed
// (DELETE /api/terminal/sessions/{id}), the process exits, or the server stops.
type ptySession struct {
	id         string
	dir        string
	agent      string // "" for a plain shell
	resume     string
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

func startPTY(cmd *exec.Cmd, dir, id, agent, resume string, persistent bool, cols, rows uint16) (*ptySession, error) {
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	if cols == 0 {
		cols = 120
	}
	if rows == 0 {
		rows = 30
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, err
	}
	ps := &ptySession{id: id, dir: dir, agent: agent, resume: resume, persistent: persistent, ptmx: ptmx, cmd: cmd, conns: map[*websocket.Conn]struct{}{}}
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
	if id != "" && !shellIDRe.MatchString(id) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31minvalid terminal id\x1b[0m\r\n"))
		return
	}

	colsVal, _ := strconv.ParseUint(q.Get("cols"), 10, 16)
	rowsVal, _ := strconv.ParseUint(q.Get("rows"), 10, 16)
	cols := uint16(colsVal)
	rows := uint16(rowsVal)
	if cols == 0 {
		cols = 120
	}
	if rows == 0 {
		rows = 30
	}

	// ?agent=claude|codex|agy[&resume=<native id>] runs that agent's CLI instead of a bare shell
	// (a login shell, so PATH from the user's profile applies); the shell stays open after it exits.
	launch := []string{shell}
	if agent != "" {
		// new=1: a brand new session without the handoff prompt; remote=1: open it with the agent's remote access on
		argv := bridge.BuildArgv(agent, bridge.LaunchOpts{
			ID:     q.Get("resume"),
			Fresh:  q.Get("new") == "1",
			Remote: q.Get("remote") == "1",
			Name:   q.Get("name"),
			Title:  q.Get("title"),
		})
		if argv == nil {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31munknown agent or invalid session id\x1b[0m\r\n"))
			return
		}
		launch = []string{shell, "-lc", bridge.ShellJoin(argv) + "; exec " + bridge.ShellJoin([]string{shell}) + " -l"}
	}

	var ps *ptySession
	if id != "" && tmuxBin() != "" {
		// the terminal itself lives in tmux; this PTY is just one attached client (killing it detaches)
		_, existed := tmuxRun("has-session", "-t", "="+id)
		if err := ensureTmuxSession(id, workDir, agent, q.Get("resume"), launch, cols, rows); err != nil {
			log.Printf("[terminal] %v", err)
			_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31mcannot start terminal session\x1b[0m\r\n"))
			return
		}
		// a freshly started, resumed agent session gets the task's title through its own rename command
		if existed != nil && agent != "" && q.Get("resume") != "" {
			if in := bridge.RenameInput(agent, q.Get("title")); in != "" {
				go typeWhenReady(id, in)
			}
		}
		attach, err := tmuxAttachCmd(id)
		if err != nil {
			return
		}
		if ps, err = startPTY(attach, workDir, id, agent, q.Get("resume"), false, cols, rows); err != nil {
			log.Printf("[terminal] pty start error: %v", err)
			return
		}
		_ = tmuxResizeWindow(id, cols, rows)
	} else if id != "" {
		ptyMu.Lock()
		ps = ptyRegistry[id]
		ptyMu.Unlock()
	}
	if ps == nil {
		ps, err = startPTY(exec.Command(launch[0], launch[1:]...), workDir, id, agent, q.Get("resume"), id != "", cols, rows)
		if err != nil {
			log.Printf("[terminal] pty start error: %v", err)
			return
		}
		if id != "" {
			ptyMu.Lock()
			ptyRegistry[id] = ps
			ptyMu.Unlock()
		}
	} else if cols > 0 && rows > 0 {
		_ = pty.Setsize(ps.ptmx, &pty.Winsize{Rows: rows, Cols: cols})
		if id != "" && tmuxBin() != "" {
			_ = tmuxResizeWindow(id, cols, rows)
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
				if id != "" && tmuxBin() != "" {
					_ = tmuxResizeWindow(id, msg.Cols, msg.Rows)
				}
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


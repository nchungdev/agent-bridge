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
	account    string
	persistent bool
	ptmx       *os.File
	cmd        *exec.Cmd

	mu    sync.Mutex
	buf   []byte
	conns map[*websocket.Conn]chan []byte
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
	buf := make([]byte, 16384)
	for {
		n, err := ps.ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			ps.mu.Lock()
			ps.buf = append(ps.buf, chunk...)
			if over := len(ps.buf) - scrollbackMax; over > 0 {
				ps.buf = append([]byte(nil), ps.buf[over:]...)
			}
			// non-blocking fan-out: a slow client or stalled connection never blocks the PTY loop
			for _, ch := range ps.conns {
				select {
				case ch <- chunk:
				default:
					// queue is full: drop the oldest unread chunk to make room for newest output
					select {
					case <-ch:
					default:
					}
					select {
					case ch <- chunk:
					default:
					}
				}
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
	for c, ch := range ps.conns {
		delete(ps.conns, c)
		close(ch)
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

func startPTY(cmd *exec.Cmd, dir, id, agent, resume, account string, persistent bool, cols, rows uint16) (*ptySession, error) {
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
	ps := &ptySession{id: id, dir: dir, agent: agent, resume: resume, account: account, persistent: persistent, ptmx: ptmx, cmd: cmd, conns: map[*websocket.Conn]chan []byte{}}
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
	account := q.Get("account")
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
		proj := q.Get("project")
		if proj == "" && workDir != "" {
			proj = bridge.FindAgyProjectID(workDir)
		}
		argv := bridge.BuildArgv(agent, bridge.LaunchOpts{
			ID:      q.Get("resume"),
			Fresh:   q.Get("new") == "1",
			Remote:  q.Get("remote") == "1",
			Name:    q.Get("name"),
			Title:   q.Get("title"),
			Project: proj,
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
		if err := ensureTmuxSession(id, workDir, agent, q.Get("resume"), account, launch, cols, rows); err != nil {
			log.Printf("[terminal] %v", err)
			_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31mcannot start terminal session\x1b[0m\r\n"))
			return
		}
		// a freshly started or resumed agent session gets the task's title through its own rename command
		if existed != nil && agent != "" && q.Get("title") != "" {
			if in := bridge.RenameInput(agent, q.Get("title")); in != "" {
				go typeWhenReady(id, in)
			}
		}
		taskID := q.Get("task_id")
		if taskID != "" && agent != "" && q.Get("resume") == "" && bridgeManager != nil {
			go func(tid, ag, dir string) {
				for i := 0; i < 15; i++ {
					time.Sleep(1 * time.Second)
					if bound := bridgeManager.AutoBindLatestUnboundSession(tid, ag, dir); bound != "" {
						break
					}
				}
			}(taskID, agent, workDir)
		}
		attach, err := tmuxAttachCmd(id)
		if err != nil {
			return
		}
		if ps, err = startPTY(attach, workDir, id, agent, q.Get("resume"), account, false, cols, rows); err != nil {
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
		ps, err = startPTY(exec.Command(launch[0], launch[1:]...), workDir, id, agent, q.Get("resume"), account, id != "", cols, rows)
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
	sendChan := make(chan []byte, 256)
	ps.mu.Lock()
	if len(ps.buf) > 0 {
		backlog := make([]byte, len(ps.buf))
		copy(backlog, ps.buf)
		sendChan <- backlog
	}
	dead := ps.dead
	if !dead {
		ps.conns[conn] = sendChan
	}
	ps.mu.Unlock()
	if dead {
		return
	}

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for chunk := range sendChan {
			batch := chunk
		drain:
			for len(batch) < 65536 {
				select {
				case next, ok := <-sendChan:
					if !ok {
						break drain
					}
					batch = append(batch, next...)
				default:
					break drain
				}
			}
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.BinaryMessage, batch); err != nil {
				return
			}
		}
	}()

	defer func() {
		ps.mu.Lock()
		if ch, ok := ps.conns[conn]; ok {
			delete(ps.conns, conn)
			close(ch)
		}
		ps.mu.Unlock()
		_ = conn.Close()
		<-writeDone
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

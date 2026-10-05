package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
		argv := bridge.LaunchArgv(agent, q.Get("resume"))
		if q.Get("new") == "1" {
			argv = bridge.FreshArgv(agent) // a brand new session: no handoff prompt
		}
		if argv == nil {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31munknown agent or invalid session id\x1b[0m\r\n"))
			return
		}
		launch = []string{shell, "-lc", bridge.ShellJoin(argv) + "; exec " + bridge.ShellJoin([]string{shell}) + " -l"}
	}

	var ps *ptySession
	if id != "" && tmuxBin() != "" {
		// the terminal itself lives in tmux; this PTY is just one attached client (killing it detaches)
		if err := ensureTmuxSession(id, workDir, agent, q.Get("resume"), launch, cols, rows); err != nil {
			log.Printf("[terminal] %v", err)
			_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31mcannot start terminal session\x1b[0m\r\n"))
			return
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

var shellIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// handleTerminalList lists the persistent terminals (shells and agent CLIs) that are still running.
func handleTerminalList(w http.ResponseWriter, r *http.Request) {
	out := []termInfo{}
	if tmuxBin() != "" {
		out = append(out, tmuxList()...)
	} else {
		ptyMu.Lock()
		for _, ps := range ptyRegistry {
			out = append(out, termInfo{ps.id, ps.dir, ps.agent, ps.resume})
		}
		ptyMu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// handleTerminalKill ends a persistent terminal.
func handleTerminalKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if shellIDRe.MatchString(id) && tmuxBin() != "" {
		tmuxKill(id)
	}
	ptyMu.Lock()
	ps := ptyRegistry[id]
	delete(ptyRegistry, id)
	ptyMu.Unlock()
	if ps != nil {
		ps.kill()
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTerminalInput sends characters/keystrokes directly into a terminal session's stdin.
func handleTerminalInput(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Input == "" {
		jsonResponse(w, map[string]any{"success": true})
		return
	}

	// Send to tmux session if running
	if shellIDRe.MatchString(id) && tmuxBin() != "" {
		_ = tmuxSendKeys(id, req.Input)
	}

	// Also send to in-process PTY
	ptyMu.Lock()
	ps := ptyRegistry[id]
	ptyMu.Unlock()
	if ps != nil && ps.ptmx != nil {
		_, _ = ps.ptmx.Write([]byte(req.Input))
	}

	jsonResponse(w, map[string]any{"success": true})
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

var pasteExt = map[string]string{
	"image/png":      ".png",
	"image/x-png":    ".png",
	"image/jpeg":     ".jpg",
	"image/pjpeg":    ".jpg",
	"image/jpg":      ".jpg",
	"image/gif":      ".gif",
	"image/webp":     ".webp",
	"image/bmp":      ".bmp",
	"image/x-ms-bmp": ".bmp",
	"image/svg+xml":  ".svg",
	"image/tiff":     ".tiff",
	"image/avif":     ".avif",
}

// handleTerminalUpload stores a pasted image so its path can be typed into the terminal
// (agent CLIs read images from a file path; the browser clipboard is not visible to them).
func handleTerminalUpload(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 25<<20))
	if err != nil || len(data) == 0 {
		http.Error(w, "invalid or empty upload body", http.StatusBadRequest)
		return
	}

	ct := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(ct, ";")[0])
	}
	mediaType = strings.ToLower(mediaType)

	ext, ok := pasteExt[mediaType]
	if !ok {
		// Sniff content type from the first 512 bytes
		sniffed := http.DetectContentType(data)
		sniffedType, _, _ := mime.ParseMediaType(sniffed)
		if e, found := pasteExt[sniffedType]; found {
			ext = e
		} else if strings.HasPrefix(sniffedType, "image/") {
			ext = "." + strings.TrimPrefix(sniffedType, "image/")
		} else {
			ext = ".png"
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dir := filepath.Join(home, ".agent-bridge", "uploads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	path := filepath.Join(dir, fmt.Sprintf("paste-%s%s", strconv.FormatInt(time.Now().UnixNano(), 36), ext))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path": path,
		"size": len(data),
	})
}

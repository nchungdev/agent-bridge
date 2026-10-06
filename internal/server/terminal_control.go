package server

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nchungdev/agent-bridge/internal/bridge"
)

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

// handleTerminalScreen returns the last lines shown by a persistent terminal (what the user sees, not the
// selection buffer): the web UI reads it to confirm that a command typed into an agent took effect.
func handleTerminalScreen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if tmuxBin() == "" || !shellIDRe.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	out, err := tmuxRun("capture-pane", "-p", "-t", "="+id, "-S", "-200")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
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

// handleTerminalExec executes a quick command and returns output.
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
		workDir = bridge.DefaultWorkspacePath()
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

// handleTerminalBuffer returns the text last selected with the mouse inside a tmux terminal. The browser
// cannot see a selection tmux made, so Cmd/Ctrl+Shift+C fetches it from here.
func handleTerminalBuffer(w http.ResponseWriter, r *http.Request) {
	if tmuxBin() == "" || !shellIDRe.MatchString(r.PathValue("id")) {
		http.NotFound(w, r)
		return
	}
	text, err := tmuxBuffer()
	if err != nil || text == "" {
		http.Error(w, "nothing selected", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain") // exact type: Safari's clipboard API rejects a charset suffix
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, text)
}

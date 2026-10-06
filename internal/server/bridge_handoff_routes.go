package server

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nchungdev/agent-bridge/internal/bridge"
)

// registerBridgeHandoffRoutes registers native sessions, handoff, GUI launcher, and remote access endpoints.
func registerBridgeHandoffRoutes(mux *http.ServeMux, bm *bridge.Manager) {
	// 1. Native sessions each CLI stored for this folder (auto context, no typing)
	mux.HandleFunc("GET /api/bridge/native-sessions", func(w http.ResponseWriter, r *http.Request) {
		ws := r.URL.Query().Get("workspace")
		if ws == "" {
			http.Error(w, "workspace required", http.StatusBadRequest)
			return
		}
		list := bm.ListNativeSessions(ws)
		if list == nil {
			list = []bridge.NativeSession{}
		}
		jsonResponse(w, list)
	})

	mux.HandleFunc("GET /api/bridge/native-session", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s := bm.GetNativeSession(q.Get("agent"), q.Get("id"), q.Get("workspace"))
		if s == nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		jsonResponse(w, s)
	})

	mux.HandleFunc("GET /api/bridge/context-usage", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		jsonResponse(w, bm.GetContextUsage(q.Get("agent"), q.Get("id"), q.Get("workspace")))
	})

	// 2. Handoff: take the source CLI's real transcript and hand it to the target CLI
	mux.HandleFunc("POST /api/bridge/handoff", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WorkspacePath string `json:"workspace_path"`
			SessionID     string `json:"session_id"`
			FromAgent     string `json:"from_agent"`
			FromNativeID  string `json:"from_native_id"`
			ToAgent       string `json:"to_agent"`
			TaskGoal      string `json:"task_goal"`
			TriggerReason string `json:"trigger_reason"`
			ExtraContext  string `json:"extra_context"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.TriggerReason == "" {
			req.TriggerReason = "manual_dashboard_switch"
		}
		if req.FromAgent != "" && req.FromNativeID != "" {
			if src := bm.GetNativeSession(req.FromAgent, req.FromNativeID, req.WorkspacePath); src != nil {
				if req.TaskGoal == "" {
					req.TaskGoal = src.Title
				}
				transcript := "### Recent Conversation (from " + req.FromAgent + " session `" + req.FromNativeID + "`)\n\n" + bridge.FormatTurns(src.Turns, 8)
				if req.ExtraContext != "" {
					transcript += "\n" + req.ExtraContext
				}
				req.ExtraContext = transcript
			}
		}
		if req.SessionID == "" {
			req.SessionID = req.FromAgent + ":" + req.FromNativeID
		}
		chk, err := bm.ExecuteHandoff(req.WorkspacePath, req.SessionID, req.FromAgent, req.ToAgent, req.TaskGoal, req.TriggerReason, req.ExtraContext)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		sameID := ""
		if req.FromAgent == req.ToAgent {
			sameID = req.FromNativeID
		}
		jsonResponse(w, map[string]any{
			"success":        true,
			"checkpoint":     chk,
			"resume_command": bridge.ResumeCommand(req.ToAgent, sameID),
		})
	})

	// 3. Open an agent's GUI: web agents give back a URL, Antigravity's IDE is launched on host
	mux.HandleFunc("POST /api/bridge/open", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Agent         string `json:"agent"`
			WorkspacePath string `json:"workspace_path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if u := bridge.GUIURL(req.Agent); u != "" {
			jsonResponse(w, map[string]any{"success": true, "url": u})
			return
		}
		if req.Agent != "agy" {
			httpError(w, os.ErrInvalid, http.StatusBadRequest)
			return
		}
		cmd := exec.Command("antigravity", req.WorkspacePath)
		if err := cmd.Start(); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		go func() { _ = cmd.Wait() }()
		jsonResponse(w, map[string]any{"success": true})
	})

	// 4. Get .agent/handoff.md content
	mux.HandleFunc("GET /api/bridge/handoff-content", func(w http.ResponseWriter, r *http.Request) {
		ws := r.URL.Query().Get("workspace")
		if ws == "" {
			ws = bridge.DefaultWorkspacePath()
		}
		path := filepath.Join(ws, ".agent", "handoff.md")
		fi, err := os.Stat(path)
		if err != nil {
			jsonResponse(w, map[string]any{"exists": false, "content": "", "path": path})
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{
			"exists":     true,
			"content":    string(data),
			"path":       path,
			"updated_at": fi.ModTime().Format(time.RFC3339),
			"size":       fi.Size(),
		})
	})

	// 5. Save/Update .agent/handoff.md content
	mux.HandleFunc("POST /api/bridge/handoff-content", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Workspace string `json:"workspace"`
			Content   string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.Workspace == "" {
			req.Workspace = bridge.DefaultWorkspacePath()
		}
		dir := filepath.Join(req.Workspace, ".agent")
		_ = os.MkdirAll(dir, 0o755)
		path := filepath.Join(dir, "handoff.md")
		if err := os.WriteFile(path, []byte(req.Content), 0o644); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true, "path": path})
	})

	// 6. Remote access: which mode an agent uses, and the commands that switch it
	mux.HandleFunc("GET /api/bridge/remote", func(w http.ResponseWriter, r *http.Request) {
		type item struct {
			Agent  string `json:"agent"`
			Mode   string `json:"mode"`
			Status string `json:"status,omitempty"`
		}
		out := []item{}
		for _, eng := range bridge.DiscoverEngines() {
			a := eng.ID
			mode := string(bridge.RemoteModeOf(a))
			if mode == "" {
				continue
			}
			it := item{Agent: a, Mode: mode}
			if a == "agy" {
				if res, err := bridge.RemoteAction(r.Context(), loginShell(), a, "status"); err == nil {
					it.Status = res.Output
				}
			}
			out = append(out, it)
		}
		jsonResponse(w, out)
	})

	mux.HandleFunc("POST /api/bridge/remote/{agent}/{action}", func(w http.ResponseWriter, r *http.Request) {
		res, err := bridge.RemoteAction(r.Context(), loginShell(), r.PathValue("agent"), r.PathValue("action"))
		if err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		jsonResponse(w, res)
	})
}

// loginShell is the shell the terminals use, so remote commands see the same PATH.
func loginShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/bash"
}

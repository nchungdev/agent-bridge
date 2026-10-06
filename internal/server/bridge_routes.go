package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nchungdev/agent-bridge/internal/bridge"
)

// RegisterBridgeRoutes binds all Agent Bridge management endpoints
func RegisterBridgeRoutes(mux *http.ServeMux, bm *bridge.Manager, db *sql.DB) {
	// 1. Workspaces / Projects
	mux.HandleFunc("GET /api/bridge/workspaces", func(w http.ResponseWriter, r *http.Request) {
		for _, p := range bm.DiscoverWorkspaces() {
			_, _ = bm.EnsureWorkspace(p)
		}
		list, err := bm.ListWorkspaces()
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, list)
	})

	mux.HandleFunc("POST /api/bridge/workspaces", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path      string `json:"path"`
			Name      string `json:"name"`
			CreateDir bool   `json:"create_dir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.Path == "" {
			http.Error(w, "path is required", http.StatusBadRequest)
			return
		}
		ws, err := bm.CreateOrUpdateWorkspace(req.Path, req.Name, req.CreateDir)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, ws)
	})

	mux.HandleFunc("PATCH /api/bridge/workspaces/name", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.ID == "" || req.Name == "" {
			http.Error(w, "id and name are required", http.StatusBadRequest)
			return
		}
		if err := bm.UpdateWorkspaceName(req.ID, req.Name); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	mux.HandleFunc("DELETE /api/bridge/workspaces/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := bm.DeleteWorkspace(id); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Engines discovery (built-in agents and auto-discovered variants like claude-me, agy-personal, codex-team)
	mux.HandleFunc("GET /api/bridge/engines", func(w http.ResponseWriter, r *http.Request) {
		engines := bridge.DiscoverEngines()
		jsonResponse(w, engines)
	})

	// File system directory browser for project folder picker
	mux.HandleFunc("GET /api/bridge/fs/directories", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("path")
		current, dirs, err := bm.BrowseDirectories(p)
		if err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		parent := filepath.Dir(current)
		if parent == current {
			parent = ""
		}
		jsonResponse(w, map[string]any{
			"current":     current,
			"parent":      parent,
			"directories": dirs,
		})
	})

	// 2. Sessions of a workspace
	// 2. Sessions of a workspace (Universal Bridge Sessions)
	mux.HandleFunc("GET /api/bridge/sessions", func(w http.ResponseWriter, r *http.Request) {
		wsPath := r.URL.Query().Get("workspace")
		wsID := r.URL.Query().Get("workspace_id")
		var sessions []bridge.BridgeSession
		var err error
		if wsPath != "" {
			sessions, err = bm.GetWorkspaceSessionsByPath(wsPath)
		} else if wsID != "" {
			sessions, err = bm.GetWorkspaceSessions(wsID, "")
		} else {
			http.Error(w, "workspace or workspace_id required", http.StatusBadRequest)
			return
		}
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		if sessions == nil {
			sessions = []bridge.BridgeSession{}
		}
		jsonResponse(w, sessions)
	})

	mux.HandleFunc("POST /api/bridge/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WorkspacePath string `json:"workspace"`
			WorkspaceID   string `json:"workspace_id"`
			Title         string `json:"title"`
			InitialAgent  string `json:"initial_agent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.WorkspaceID == "" && req.WorkspacePath != "" {
			ws, err := bm.EnsureWorkspace(req.WorkspacePath)
			if err != nil {
				httpError(w, err, http.StatusInternalServerError)
				return
			}
			req.WorkspaceID = ws.ID
		}
		if req.Title == "" {
			req.Title = "Phiên làm việc mới"
		}
		s, err := bm.CreateSession(req.WorkspaceID, req.Title, req.InitialAgent)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, s)
	})

	// Bind a native agent session to a universal bridge session
	mux.HandleFunc("POST /api/bridge/sessions/bind", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			BridgeSessionID string `json:"bridge_session_id"`
			Agent           string `json:"agent"`
			NativeSessionID string `json:"native_session_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.BridgeSessionID == "" || req.Agent == "" || req.NativeSessionID == "" {
			http.Error(w, "missing required fields", http.StatusBadRequest)
			return
		}
		if err := bm.BindAgent(req.BridgeSessionID, req.Agent, req.NativeSessionID); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	mux.HandleFunc("PATCH /api/bridge/sessions/title", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if err := bm.UpdateSessionTitle(req.ID, req.Title); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	mux.HandleFunc("DELETE /api/bridge/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := bm.DeleteSession(id); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Smart handoff sync when opening an agent within a task
	mux.HandleFunc("POST /api/bridge/sync-handoff", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Workspace   string `json:"workspace"`
			TaskID      string `json:"task_id"`
			TargetAgent string `json:"target_agent"`
			SourceAgent string `json:"source_agent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.Workspace == "" || req.TaskID == "" || req.TargetAgent == "" {
			http.Error(w, "missing required fields (workspace, task_id, target_agent)", http.StatusBadRequest)
			return
		}
		synced, msg, err := bm.SyncHandoff(req.Workspace, req.TaskID, req.TargetAgent, req.SourceAgent)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{
			"synced":  synced,
			"message": msg,
		})
	})

	// 3. Context State & Git Diff
	mux.HandleFunc("GET /api/bridge/state", func(w http.ResponseWriter, r *http.Request) {
		wsPath := r.URL.Query().Get("workspace")
		if wsPath == "" {
			wsPath = "/home/chungnh/AI Workspace"
		}
		gitSummary, modFiles := bm.CaptureWorkspaceState(wsPath)
		jsonResponse(w, map[string]any{
			"workspace":      wsPath,
			"git_summary":    gitSummary,
			"modified_files": modFiles,
		})
	})

	// 4. Native sessions each CLI stored for this folder (auto context, no typing)
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

	// 5. Handoff: take the source CLI's real transcript and hand it to the target CLI
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

	// 6. Open an agent's GUI: web agents give back a URL, Antigravity's IDE is launched on this machine
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

	// 7. Get .agent/handoff.md content
	mux.HandleFunc("GET /api/bridge/handoff-content", func(w http.ResponseWriter, r *http.Request) {
		ws := r.URL.Query().Get("workspace")
		if ws == "" {
			ws = "/home/chungnh/AI Workspace"
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

	// 8. Save/Update .agent/handoff.md content
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
			req.Workspace = "/home/chungnh/AI Workspace"
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

	// 7. Remote access (web / mobile apps of each agent): which mode an agent uses, and the commands that switch it
	mux.HandleFunc("GET /api/bridge/remote", func(w http.ResponseWriter, r *http.Request) {
		type item struct {
			Agent  string `json:"agent"`
			Mode   string `json:"mode"`
			Status string `json:"status,omitempty"` // only agents that can report it (agy)
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

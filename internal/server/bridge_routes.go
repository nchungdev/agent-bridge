package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"

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
			wsPath = bridge.DefaultWorkspacePath()
		}
		gitSummary, modFiles := bm.CaptureWorkspaceState(wsPath)
		jsonResponse(w, map[string]any{
			"workspace":      wsPath,
			"git_summary":    gitSummary,
			"modified_files": modFiles,
		})
	})

	// 4. Handoff, Native Sessions, GUI & Remote Access
	registerBridgeHandoffRoutes(mux, bm)
}

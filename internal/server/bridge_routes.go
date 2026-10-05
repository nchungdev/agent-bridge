package server

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/nchungdev/agent-hub/internal/bridge"
)

// RegisterBridgeRoutes binds all Agent Bridge management endpoints
func RegisterBridgeRoutes(mux *http.ServeMux, bm *bridge.Manager, db *sql.DB) {
	// 1. Workspaces
	mux.HandleFunc("GET /api/bridge/workspaces", func(w http.ResponseWriter, r *http.Request) {
		list, err := bm.ListWorkspaces()
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, list)
	})

	mux.HandleFunc("POST /api/bridge/workspaces", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		ws, err := bm.EnsureWorkspace(req.Path)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, ws)
	})

	// 2. Sessions of a workspace
	mux.HandleFunc("GET /api/bridge/sessions", func(w http.ResponseWriter, r *http.Request) {
		wsID := r.URL.Query().Get("workspace_id")
		if wsID == "" {
			http.Error(w, "workspace_id required", http.StatusBadRequest)
			return
		}
		sessions, err := bm.GetWorkspaceSessions(wsID)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, sessions)
	})

	mux.HandleFunc("POST /api/bridge/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WorkspaceID  string `json:"workspace_id"`
			Title        string `json:"title"`
			InitialAgent string `json:"initial_agent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		s, err := bm.CreateSession(req.WorkspaceID, req.Title, req.InitialAgent)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, s)
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

	// 4. Trigger Handoff (Switch Engine with Context)
	mux.HandleFunc("POST /api/bridge/handoff", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WorkspacePath string `json:"workspace_path"`
			SessionID     string `json:"session_id"`
			FromAgent     string `json:"from_agent"`
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
		chk, err := bm.ExecuteHandoff(
			req.WorkspacePath,
			req.SessionID,
			req.FromAgent,
			req.ToAgent,
			req.TaskGoal,
			req.TriggerReason,
			req.ExtraContext,
		)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{
			"success":    true,
			"checkpoint": chk,
		})
	})
}

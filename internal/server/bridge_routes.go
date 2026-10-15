package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/nchungdev/agent-bridge/internal/bridge"
)

// RegisterBridgeRoutes binds all Agent Bridge management endpoints
var bridgeManager *bridge.Manager

func RegisterBridgeRoutes(mux *http.ServeMux, bm *bridge.Manager, db *sql.DB) {
	bridgeManager = bm
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
			WorkspacePath string           `json:"workspace"`
			WorkspaceID   string           `json:"workspace_id"`
			Title         string           `json:"title"`
			InitialAgent  string           `json:"initial_agent"`
			KanbanStatus  string           `json:"kanban_status"`
			Subtasks      []bridge.Subtask `json:"subtasks"`
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
		s, err := bm.CreateSessionWithKanban(req.WorkspaceID, req.Title, req.InitialAgent, req.KanbanStatus, req.Subtasks)
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
		safeID := req.ID
		if len(safeID) > 18 {
			safeID = safeID[:18]
		}
		for _, ag := range []string{"agy", "claude", "codex"} {
			termKey := fmt.Sprintf("term-%s-%s", safeID, ag)
			if in := bridge.RenameInput(ag, req.Title); in != "" {
				_ = tmuxSendKeys(termKey, in)
			}
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

	// --- Kanban & Autonomous Task Routes ---

	// Update task Kanban column status and order
	mux.HandleFunc("PATCH /api/bridge/tasks/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			Status   string `json:"status"`
			OrderIdx int    `json:"order_idx"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.Status == "" {
			http.Error(w, "status is required", http.StatusBadRequest)
			return
		}
		if err := bm.UpdateTaskKanban(id, req.Status, req.OrderIdx); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Assign task to an agent (agy, claude, codex)
	mux.HandleFunc("PATCH /api/bridge/tasks/{id}/assign", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			Agent string `json:"agent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.Agent == "" {
			http.Error(w, "agent is required", http.StatusBadRequest)
			return
		}
		if err := bm.UpdateTaskAgent(id, req.Agent); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Update full list of subtasks
	mux.HandleFunc("PATCH /api/bridge/tasks/{id}/subtasks", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			Subtasks []bridge.Subtask `json:"subtasks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if err := bm.UpdateTaskSubtasks(id, req.Subtasks); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Add a single subtask
	mux.HandleFunc("POST /api/bridge/tasks/{id}/subtasks/add", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		subtask, err := bm.AddSubtask(id, req.Title)
		if err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		jsonResponse(w, map[string]any{"success": true, "subtask": subtask})
	})

	// Toggle subtask completion
	mux.HandleFunc("POST /api/bridge/tasks/{id}/subtasks/toggle", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			SubtaskID string `json:"subtask_id"`
			Done      bool   `json:"done"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.SubtaskID == "" {
			http.Error(w, "subtask_id is required", http.StatusBadRequest)
			return
		}
		if err := bm.ToggleSubtask(id, req.SubtaskID, req.Done); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Delete a subtask
	mux.HandleFunc("DELETE /api/bridge/tasks/{id}/subtasks/{subtaskId}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		subtaskId := r.PathValue("subtaskId")
		if err := bm.DeleteSubtask(id, subtaskId); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Decompose task: accepts array of string titles and creates subtasks
	mux.HandleFunc("POST /api/bridge/tasks/{id}/decompose", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			Titles []string `json:"titles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		var newSubtasks []bridge.Subtask
		for _, t := range req.Titles {
			if strings.TrimSpace(t) != "" {
				newSubtasks = append(newSubtasks, bridge.Subtask{
					ID:    fmt.Sprintf("st_%d", time.Now().UnixNano()),
					Title: strings.TrimSpace(t),
					Done:  false,
				})
			}
		}
		if len(newSubtasks) > 0 {
			if err := bm.UpdateTaskSubtasks(id, newSubtasks); err != nil {
				httpError(w, err, http.StatusInternalServerError)
				return
			}
		}
		jsonResponse(w, map[string]any{"success": true, "subtasks": newSubtasks})
	})

	// Update task result summary (e.g. from agent when finished or when user reviews)
	mux.HandleFunc("PATCH /api/bridge/tasks/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			Summary string `json:"summary"`
			Status  string `json:"status"` // e.g. "done" or "review"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if err := bm.UpdateTaskResult(id, req.Summary, req.Status); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Cleanup all todo tasks in a workspace
	mux.HandleFunc("POST /api/bridge/tasks/cleanup-todo", func(w http.ResponseWriter, r *http.Request) {
		ws := r.URL.Query().Get("workspace")
		if ws == "" {
			http.Error(w, "workspace is required", http.StatusBadRequest)
			return
		}
		deletedCount, err := bm.CleanupTodoTasks(ws)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true, "deleted_count": deletedCount})
	})

	// Autonomous Agent Report endpoint: allows an agent daemon or script to report status, subtasks, and outcome
	mux.HandleFunc("POST /api/bridge/agent/report", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TaskID   string           `json:"task_id"`
			Status   string           `json:"status"` // "in_progress", "review", "done"
			Summary  string           `json:"summary"`
			Subtasks []bridge.Subtask `json:"subtasks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.TaskID == "" {
			http.Error(w, "task_id is required", http.StatusBadRequest)
			return
		}
		if req.Status != "" {
			_ = bm.UpdateTaskKanban(req.TaskID, req.Status, 0)
		}
		if req.Summary != "" {
			_ = bm.UpdateTaskResult(req.TaskID, req.Summary, req.Status)
		}
		if len(req.Subtasks) > 0 {
			_ = bm.UpdateTaskSubtasks(req.TaskID, req.Subtasks)
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

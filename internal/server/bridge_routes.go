package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nchungdev/agent-bridge/internal/bridge"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkHtml "github.com/yuin/goldmark/renderer/html"
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
		if s != nil && s.KanbanStatus == "in_progress" {
			if req.WorkspacePath != "" {
				s.Workspace = req.WorkspacePath
			}
			go autoDispatchTask(s)
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
		killTaskTmux(id)
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
		if req.Status == "in_progress" {
			if task, _ := bm.GetSession(id); task != nil {
				go autoDispatchTask(task)
			}
		} else if req.Status == "done" {
			killTaskTmux(id)
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
		if task, _ := bm.GetSession(id); task != nil && task.KanbanStatus == "in_progress" {
			task.CurrentAgent = req.Agent
			go autoDispatchTask(task)
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Trigger or restart autonomous execution of a task
	mux.HandleFunc("POST /api/bridge/tasks/{id}/dispatch", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		task, err := bm.GetSession(id)
		if err != nil || task == nil {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		_ = bm.UpdateTaskKanban(id, "in_progress", task.OrderIdx)
		task.KanbanStatus = "in_progress"
		termKey, err := autoDispatchTask(task)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true, "term_key": termKey})
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
		if req.Status == "done" {
			killTaskTmux(id)
		}
		// Persist report file automatically
		if task, _ := bm.GetSession(id); task != nil {
			task.ResultSummary = req.Summary
			if req.Status != "" {
				task.KanbanStatus = req.Status
			}
			wsDir := task.Workspace
			if wsDir == "" && bm != nil {
				if ws, _ := bm.GetWorkspaceByID(task.WorkspaceID); ws != nil {
					wsDir = ws.Path
				}
			}
			if wsDir != "" {
				reportsDir := filepath.Join(wsDir, ".agent", "reports")
				_ = os.MkdirAll(reportsDir, 0755)
				safeID := getSafeID(task.ID)
				md := buildTaskReportMarkdown(task, wsDir, bm)
				_ = os.WriteFile(filepath.Join(reportsDir, safeID+".md"), []byte(md), 0644)
			}
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Get task report file (simply open .md file directly in browser tab, or download if download=1)
	handleTaskReport := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		id = strings.TrimSuffix(id, ".md")
		task, err := bm.GetSession(id)
		if err != nil || task == nil {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}

		wsDir := task.Workspace
		if wsDir == "" && bm != nil {
			if ws, _ := bm.GetWorkspaceByID(task.WorkspaceID); ws != nil {
				wsDir = ws.Path
			}
		}
		if wsDir == "" {
			wsDir = bridge.DefaultWorkspacePath()
		}

		reportsDir := filepath.Join(wsDir, ".agent", "reports")
		_ = os.MkdirAll(reportsDir, 0755)

		safeID := getSafeID(task.ID)
		reportPath := filepath.Join(reportsDir, safeID+".md")
		md := buildTaskReportMarkdown(task, wsDir, bm)
		_ = os.WriteFile(reportPath, []byte(md), 0644)

		if r.URL.Query().Get("download") == "1" || r.URL.Query().Get("raw") == "1" {
			filename := fmt.Sprintf("report-%s.md", safeID)
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.Write([]byte(md))
			return
		}

		// Convert markdown to clean HTML of the content alone for direct viewing in browser tab
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(renderTaskReportHTML(task.Title, md)))
	}
	mux.HandleFunc("GET /api/bridge/tasks/{id}/report", handleTaskReport)
	mux.HandleFunc("GET /api/bridge/tasks/{id}/report.md", handleTaskReport)

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

	// Project Memory Ledger: GET .agent/memory.md
	mux.HandleFunc("GET /api/bridge/workspace/memory", func(w http.ResponseWriter, r *http.Request) {
		ws := r.URL.Query().Get("workspace")
		if ws == "" {
			http.Error(w, "workspace is required", http.StatusBadRequest)
			return
		}
		content, err := bm.GetProjectMemory(ws)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"content": content})
	})

	// Project Memory Ledger: PUT .agent/memory.md
	mux.HandleFunc("PUT /api/bridge/workspace/memory", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Workspace string `json:"workspace"`
			Content   string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.Workspace == "" {
			http.Error(w, "workspace is required", http.StatusBadRequest)
			return
		}
		if err := bm.UpdateProjectMemory(req.Workspace, req.Content); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]any{"success": true})
	})

	// Smart Ingest: auto-routes instruction into active task if related or creates new task
	mux.HandleFunc("POST /api/bridge/tasks/smart-ingest", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Workspace string `json:"workspace"`
			Title     string `json:"title"`
			Agent     string `json:"agent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.Workspace == "" || req.Title == "" {
			http.Error(w, "workspace and title are required", http.StatusBadRequest)
			return
		}
		sess, wasMerged, subtask, err := bm.SmartIngestTask(req.Workspace, req.Title, req.Agent)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		if sess != nil {
			sess.Workspace = req.Workspace
			if !wasMerged && sess.KanbanStatus == "in_progress" {
				go autoDispatchTask(sess)
			} else if wasMerged && subtask != nil {
				safeID := sess.ID
				if len(safeID) > 18 {
					safeID = safeID[:18]
				}
				safeID = strings.Map(func(r rune) rune {
					if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
						return r
					}
					return -1
				}, safeID)
				termKey := fmt.Sprintf("term-%s-%s", safeID, sess.CurrentAgent)
				_ = tmuxSendKeys(termKey, fmt.Sprintf("Yêu cầu bổ sung: %s\n", subtask.Title))
			}
		}
		jsonResponse(w, map[string]any{
			"success":    true,
			"was_merged": wasMerged,
			"session":    sess,
			"subtask":    subtask,
		})
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

// killTaskTmux kills any background tmux sessions running for this task
func killTaskTmux(taskID string) {
	safeID := taskID
	if len(safeID) > 18 {
		safeID = safeID[:18]
	}
	safeID = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return -1
	}, safeID)
	for _, ag := range []string{"agy", "claude", "codex", "sh"} {
		termKey := fmt.Sprintf("term-%s-%s", safeID, ag)
		tmuxKill(termKey)
	}
}

// autoDispatchTask starts an autonomous background tmux session for an in_progress task and injects its prompt.
func autoDispatchTask(task *bridge.BridgeSession) (string, error) {
	if task == nil {
		return "", fmt.Errorf("task is nil")
	}
	agent := task.CurrentAgent
	if agent == "" {
		agent = "agy"
	}
	wsDir := task.Workspace
	if wsDir == "" && bridgeManager != nil {
		if ws, _ := bridgeManager.GetWorkspaceByID(task.WorkspaceID); ws != nil {
			wsDir = ws.Path
		}
	}
	if wsDir == "" {
		return "", fmt.Errorf("no workspace directory for task %s", task.ID)
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	safeID := task.ID
	if len(safeID) > 18 {
		safeID = safeID[:18]
	}
	safeID = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return -1
	}, safeID)
	termKey := fmt.Sprintf("term-%s-%s", safeID, agent)

	// Check if tmux session already exists
	if _, err := tmuxRun("has-session", "-t", "="+termKey); err == nil {
		// Session is already running. Return key
		return termKey, nil
	}

	proj := ""
	if bridge.BaseAgent(agent) == "agy" {
		proj = bridge.FindAgyProjectID(wsDir)
	}

	boundNativeID := ""
	if task.Bindings != nil {
		boundNativeID = task.Bindings[agent]
	}

	argv := bridge.BuildArgv(agent, bridge.LaunchOpts{
		ID:      boundNativeID,
		Fresh:   boundNativeID == "",
		Remote:  true,
		Title:   task.Title,
		Project: proj,
	})
	if argv == nil {
		argv = []string{agent}
	}

	launch := []string{shell, "-lc", bridge.ShellJoin(argv) + "; exec " + bridge.ShellJoin([]string{shell}) + " -l"}

	// Start tmux session in background
	if err := ensureTmuxSession(termKey, wsDir, agent, boundNativeID, "default", launch, 120, 30); err != nil {
		return termKey, fmt.Errorf("ensureTmuxSession: %w", err)
	}

	// Prepare prompt
	prompt := buildTaskPrompt(task)
	if prompt != "" {
		go typeWhenReady(termKey, prompt+"\n")
	}

	// Auto-bind newly created native agent session
	if boundNativeID == "" && bridgeManager != nil {
		go func(tid, ag, dir string) {
			for i := 0; i < 20; i++ {
				time.Sleep(1 * time.Second)
				if bound := bridgeManager.AutoBindLatestUnboundSession(tid, ag, dir); bound != "" {
					break
				}
			}
		}(task.ID, agent, wsDir)
	}

	return termKey, nil
}

func buildTaskPrompt(task *bridge.BridgeSession) string {
	if task == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(task.Title)
	if len(task.Subtasks) > 0 {
		sb.WriteString("\nCác bước cần thực hiện:\n")
		for _, st := range task.Subtasks {
			if !st.Done {
				sb.WriteString(fmt.Sprintf("- %s\n", st.Title))
			}
		}
	}
	return sb.String()
}

func getSafeID(id string) string {
	safeID := id
	if len(safeID) > 18 {
		safeID = safeID[:18]
	}
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return -1
	}, safeID)
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func cleanConsoleOutput(out string) string {
	out = ansiRegex.ReplaceAllString(out, "")
	lines := strings.Split(out, "\n")
	var result []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.Contains(trimmed, "? for shortcuts") || strings.Contains(trimmed, "Gemini 3.8 Flash") {
			continue
		}
		if trimmed == ">" || (strings.HasPrefix(trimmed, "───") && len(trimmed) > 40) {
			continue
		}
		result = append(result, l)
	}
	for len(result) > 0 && strings.TrimSpace(result[0]) == "" {
		result = result[1:]
	}
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}
	return strings.Join(result, "\n")
}

func agentDisplayName(agent string) string {
	a := strings.ToLower(strings.TrimSpace(agent))
	if strings.HasPrefix(a, "claude") {
		return "Claude Code (Anthropic)"
	}
	if strings.HasPrefix(a, "codex") {
		return "Codex CLI (OpenAI)"
	}
	if strings.HasPrefix(a, "agy") || strings.HasPrefix(a, "antigravity") {
		return "Antigravity (Google)"
	}
	if agent == "" {
		return "Chưa gán agent"
	}
	return agent
}

func statusDisplayName(status string) string {
	switch status {
	case "todo":
		return "To Do (Cần làm)"
	case "in_progress":
		return "In Progress (Đang chạy)"
	case "review":
		return "Review & Handoff (Chờ duyệt)"
	case "done":
		return "Done (Hoàn thành)"
	default:
		return status
	}
}

func buildTaskReportMarkdown(task *bridge.BridgeSession, wsDir string, bm *bridge.Manager) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# BÁO CÁO KẾT QUẢ: %s\n\n", task.Title))
	sb.WriteString(fmt.Sprintf("- **Mã Task:** `%s`\n", task.ID))
	sb.WriteString(fmt.Sprintf("- **Không gian làm việc (Workspace):** `%s`\n", wsDir))
	sb.WriteString(fmt.Sprintf("- **Agent thực hiện:** %s\n", agentDisplayName(task.CurrentAgent)))
	sb.WriteString(fmt.Sprintf("- **Trạng thái:** %s\n", statusDisplayName(task.KanbanStatus)))
	sb.WriteString(fmt.Sprintf("- **Thời gian cập nhật:** %s\n\n", task.UpdatedAt.Format("15:04:05 02/01/2006")))
	sb.WriteString("---\n\n")

	sb.WriteString("## 1. Kết quả thực hiện (Result Summary)\n\n")
	if strings.TrimSpace(task.ResultSummary) != "" {
		sb.WriteString(task.ResultSummary)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("*(Chưa có bản tóm tắt kết quả)*\n\n")
	}

	if len(task.Subtasks) > 0 {
		sb.WriteString("## 2. Danh sách Subtasks\n\n")
		for _, st := range task.Subtasks {
			check := " "
			if st.Done {
				check = "x"
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", check, st.Title))
		}
		sb.WriteString("\n")
	}

	// Capture detailed execution output from tmux pane if available
	safeID := getSafeID(task.ID)
	termKey := fmt.Sprintf("term-%s-%s", safeID, task.CurrentAgent)
	if out, err := tmuxRun("capture-pane", "-t", termKey, "-p", "-S", "-300"); err == nil && strings.TrimSpace(string(out)) != "" {
		cleaned := cleanConsoleOutput(string(out))
		if len(cleaned) > 50 {
			sb.WriteString("## 3. Nhật ký thực thi từ Agent (Execution Details)\n\n")
			sb.WriteString(cleaned)
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("*Báo cáo được tổng hợp tự động bởi Agent Bridge vào lúc %s*\n", time.Now().Format("15:04:05 02/01/2006")))
	return sb.String()
}

var mdParser = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Table,
		extension.TaskList,
		extension.Strikethrough,
		extension.Linkify,
	),
	goldmark.WithRendererOptions(
		goldmarkHtml.WithUnsafe(),
	),
)

func convertMarkdownToHTML(src string) string {
	var buf bytes.Buffer
	if err := mdParser.Convert([]byte(src), &buf); err != nil {
		return html.EscapeString(src)
	}
	return buf.String()
}

func renderTaskReportHTML(title, mdContent string) string {
	convertedBodyHTML := convertMarkdownToHTML(mdContent)
	safeTitle := html.EscapeString(title)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="vi">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #0d1117;
      --text: #e6edf3;
      --border: #30363d;
      --code-bg: #161b22;
      --quote-border: #388bfd;
      --quote-bg: rgba(56, 139, 253, 0.1);
      --table-even: #161b22;
      --link: #58a6ff;
    }
    @media (prefers-color-scheme: light) {
      :root {
        --bg: #ffffff;
        --text: #1f2328;
        --border: #d0d7de;
        --code-bg: #f6f8fa;
        --quote-border: #0969da;
        --quote-bg: rgba(9, 105, 218, 0.05);
        --table-even: #f6f8fa;
        --link: #0969da;
      }
    }
    * { box-sizing: border-box; }
    body {
      background-color: var(--bg);
      color: var(--text);
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", Helvetica, Arial, sans-serif;
      font-size: 15px;
      line-height: 1.65;
      margin: 0;
      padding: 40px 24px 80px 24px;
      word-wrap: break-word;
    }
    .markdown-body {
      max-width: 860px;
      margin: 0 auto;
    }
    .markdown-body h1, .markdown-body h2, .markdown-body h3, .markdown-body h4 {
      font-weight: 600;
      line-height: 1.3;
      margin-top: 28px;
      margin-bottom: 14px;
    }
    .markdown-body h1:first-child {
      margin-top: 0;
    }
    .markdown-body h1 {
      font-size: 24px;
      padding-bottom: 8px;
      border-bottom: 1px solid var(--border);
    }
    .markdown-body h2 {
      font-size: 18px;
      padding-bottom: 6px;
      border-bottom: 1px solid var(--border);
    }
    .markdown-body h3 { font-size: 15px; }
    .markdown-body p { margin-top: 0; margin-bottom: 14px; }
    .markdown-body ul, .markdown-body ol {
      padding-left: 24px;
      margin-top: 0;
      margin-bottom: 14px;
    }
    .markdown-body li { margin-top: 4px; }
    .markdown-body input[type="checkbox"] {
      margin-right: 8px;
      vertical-align: middle;
    }
    .markdown-body blockquote {
      border-left: 4px solid var(--quote-border);
      padding: 8px 16px;
      background: var(--quote-bg);
      margin: 14px 0;
      border-radius: 4px;
    }
    .markdown-body code {
      background: var(--code-bg);
      padding: 2px 6px;
      border-radius: 6px;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 85%%;
    }
    .markdown-body pre {
      background: var(--code-bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 14px;
      overflow-x: auto;
      margin: 14px 0;
    }
    .markdown-body pre code {
      background: transparent;
      padding: 0;
      font-size: 13px;
      line-height: 1.5;
    }
    .markdown-body table {
      border-collapse: collapse;
      width: 100%%;
      margin: 14px 0;
    }
    .markdown-body th, .markdown-body td {
      border: 1px solid var(--border);
      padding: 8px 12px;
    }
    .markdown-body th {
      background: var(--code-bg);
      font-weight: 600;
    }
    .markdown-body tr:nth-child(even) {
      background: var(--table-even);
    }
    .markdown-body hr {
      border: 0;
      height: 1px;
      background: var(--border);
      margin: 24px 0;
    }
    .markdown-body a {
      color: var(--link);
      text-decoration: none;
    }
    .markdown-body a:hover {
      text-decoration: underline;
    }
    @media print {
      body { background: white; color: black; padding: 0; }
      .markdown-body { max-width: 100%%; margin: 0; }
      .markdown-body code { background: #f6f8fa; color: black; }
    }
  </style>
</head>
<body class="markdown-body">
%s
</body>
</html>`,
		safeTitle,
		convertedBodyHTML,
	)
}



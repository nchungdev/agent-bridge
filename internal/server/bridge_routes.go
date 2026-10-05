package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"

	"github.com/nchungdev/agent-bridge/internal/bridge"
)

// RegisterBridgeRoutes binds all Agent Bridge management endpoints
func RegisterBridgeRoutes(mux *http.ServeMux, bm *bridge.Manager, db *sql.DB) {
	// 1. Workspaces
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
}

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nchungdev/agent-hub/internal/agent"
	"github.com/nchungdev/agent-hub/internal/session"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     sameOrigin,
}

// Routes registers all HTTP and WebSocket endpoints.
func Routes(mux *http.ServeMux, hub *Hub, sm *session.Manager, dispatcher *agent.Dispatcher) {
	// REST API
	mux.HandleFunc("GET /api/sessions", handleListSessions(sm))
	mux.HandleFunc("POST /api/sessions", handleCreateSession(sm))
	mux.HandleFunc("GET /api/sessions/{id}", handleGetSession(sm))
	mux.HandleFunc("DELETE /api/sessions/{id}", handleDeleteSession(sm))
	mux.HandleFunc("GET /api/sessions/{id}/messages", handleGetMessages(sm))
	mux.HandleFunc("GET /api/agents", handleListAgents(dispatcher))
	mux.HandleFunc("POST /api/agents/check", handleCheckAgent())
	mux.HandleFunc("POST /api/agents/install", handleInstallAgent())
	mux.HandleFunc("POST /api/agents/register", handleRegisterAgent(dispatcher))
	mux.HandleFunc("POST /api/agents/auth-action", handleAgentAuthAction())
	mux.HandleFunc("POST /api/agents/codex-login", handleCodexDeviceLogin())

	// Antigravity native data endpoints
	mux.HandleFunc("GET /api/antigravity/projects", handleGetAntigravityProjects)
	mux.HandleFunc("GET /api/antigravity/conversations/{id}/messages", handleGetAntigravityMessages)
	mux.HandleFunc("GET /api/media", handleGetMediaFile)
	mux.HandleFunc("POST /api/upload", handleUploadFile)

	// Workspace File Explorer & Git Diff endpoints
	mux.HandleFunc("GET /api/fs/tree", handleGetFileTree)
	mux.HandleFunc("GET /api/fs/content", handleGetFileContent)
	mux.HandleFunc("GET /api/git/diff", handleGetGitDiff)

	// Interactive Terminal endpoints (disable with AGENT_HUB_TERMINAL=0)
	if terminalEnabled() {
		mux.HandleFunc("/ws/terminal", handleTerminalWS)
		mux.HandleFunc("POST /api/terminal/exec", handleTerminalExec)
		mux.HandleFunc("GET /api/terminal/sessions", handleTerminalList)
		mux.HandleFunc("DELETE /api/terminal/sessions/{id}", handleTerminalKill)
	}

	// WebSocket
	mux.HandleFunc("/ws", handleWebSocket(hub, sm, dispatcher))
}

// --- REST Handlers ---

func handleListSessions(sm *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessions, err := sm.ListSessions()
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		if sessions == nil {
			sessions = []session.Session{}
		}
		jsonResponse(w, sessions)
	}
}

func handleCreateSession(sm *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name      string `json:"name"`
			Workspace string `json:"workspace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			req.Name = "New Session"
		}
		s, err := sm.CreateSession(req.Name, req.Workspace)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonResponse(w, s)
	}
}

func handleGetSession(sm *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		s, err := sm.GetSession(id)
		if err != nil {
			httpError(w, err, http.StatusNotFound)
			return
		}
		jsonResponse(w, s)
	}
}

func handleDeleteSession(sm *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := sm.DeleteSession(id); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleGetMessages(sm *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		msgs, err := sm.GetMessages(id, 200)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		if msgs == nil {
			msgs = []session.Message{}
		}
		jsonResponse(w, msgs)
	}
}

func handleListAgents(dispatcher *agent.Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents := dispatcher.ListAgents()
		jsonResponse(w, agents)
	}
}

func handleCheckAgent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Binary  string `json:"binary"`
			Command string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		binary := strings.TrimSpace(req.Binary)
		if binary == "" {
			binary = strings.TrimSpace(req.Command)
		}
		if binary == "" {
			jsonResponse(w, map[string]any{"found": false, "error": "binary or command is empty"})
			return
		}

		fullPath := ""
		if strings.HasPrefix(binary, "/") || strings.HasPrefix(binary, "./") || strings.HasPrefix(binary, "~/") {
			if strings.HasPrefix(binary, "~/") {
				home, _ := os.UserHomeDir()
				binary = filepath.Join(home, binary[2:])
			}
			if info, err := os.Stat(binary); err == nil && !info.IsDir() {
				fullPath = binary
			}
		} else {
			pathEnv := os.Getenv("PATH")
			for _, dir := range filepath.SplitList(pathEnv) {
				candidate := filepath.Join(dir, binary)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					fullPath = candidate
					break
				}
			}
		}

		// Check authentication status on system
		home, _ := os.UserHomeDir()
		authStatus := "Not authenticated"
		hasAuth := false
		authMethod := "none"

		binLower := strings.ToLower(binary)
		if strings.Contains(binLower, "claude") {
			// First check environment key
			if os.Getenv("ANTHROPIC_API_KEY") != "" {
				hasAuth = true
				authMethod = "env"
				authStatus = "ANTHROPIC_API_KEY detected in environment"
			} else if fullPath != "" {
				// Query claude auth status directly
				cmd := exec.Command(fullPath, "auth", "status")
				if out, err := cmd.Output(); err == nil || len(out) > 0 {
					var statusData struct {
						LoggedIn   bool   `json:"loggedIn"`
						AuthMethod string `json:"authMethod"`
					}
					// Find first JSON object in output
					outStr := string(out)
					if idx := strings.Index(outStr, "{"); idx >= 0 {
						if endIdx := strings.LastIndex(outStr, "}"); endIdx > idx {
							json.Unmarshal([]byte(outStr[idx:endIdx+1]), &statusData)
						}
					}
					if statusData.LoggedIn {
						hasAuth = true
						authMethod = "oauth"
						authStatus = "Logged in (" + statusData.AuthMethod + ")"
					} else {
						hasAuth = false
						authMethod = "oauth"
						authStatus = "Not logged in. Click 'Open Terminal' or run 'claude login'"
					}
				} else {
					hasAuth = false
					authMethod = "oauth"
					authStatus = "Not logged in. Run 'claude login' in terminal"
				}
			} else {
				hasAuth = false
				authMethod = "none"
				authStatus = "Binary not installed"
			}
		} else if strings.Contains(binLower, "antigravity") || strings.Contains(binLower, "agy") {
			oauthJson := filepath.Join(home, ".gemini", "oauth_creds.json")
			if _, err := os.Stat(oauthJson); err == nil {
				hasAuth = true
				authMethod = "oauth"
				authStatus = "OAuth session active (~/.gemini/oauth_creds.json)"
			} else {
				hasAuth = true
				authMethod = "oauth"
				authStatus = "Google Account / ADC credentials active"
			}
		} else if strings.Contains(binLower, "codex") {
			if os.Getenv("OPENAI_API_KEY") != "" {
				hasAuth = true
				authMethod = "env"
				authStatus = "OPENAI_API_KEY detected in environment"
			} else {
				authStatus = "No session or key detected"
			}
		} else {
			hasAuth = true
			authMethod = "env"
			authStatus = "Inherited from terminal environment"
		}

		if fullPath != "" {
			jsonResponse(w, map[string]any{
				"found":       true,
				"path":        fullPath,
				"has_auth":    hasAuth,
				"auth_method": authMethod,
				"auth_status": authStatus,
			})
		} else {
			jsonResponse(w, map[string]any{
				"found":       false,
				"error":       fmt.Sprintf("Binary '%s' not found in system PATH", binary),
				"has_auth":    hasAuth,
				"auth_method": authMethod,
				"auth_status": authStatus,
			})
		}
	}
}

func handleInstallAgent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Command string `json:"command"`
			Binary  string `json:"binary"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		installCmd := strings.TrimSpace(req.Command)
		binaryName := strings.TrimSpace(req.Binary)
		if installCmd == "" {
			jsonResponse(w, map[string]any{"success": false, "error": "install command is empty"})
			return
		}

		// Auto-escalate with sudo for global package managers that need root
		needsSudo := false
		if strings.Contains(installCmd, "npm install -g") ||
			strings.Contains(installCmd, "npm i -g") {
			needsSudo = true
		} else if strings.HasPrefix(installCmd, "pip install") && !strings.Contains(installCmd, "--user") {
			needsSudo = true
		}
		if needsSudo && !strings.HasPrefix(installCmd, "sudo ") {
			installCmd = "sudo " + installCmd
		}

		// Run install command with 120s timeout
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "bash", "-c", installCmd)
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		output, err := cmd.CombinedOutput()

		outStr := string(output)
		if err != nil {
			jsonResponse(w, map[string]any{
				"success": false,
				"error":   err.Error(),
				"output":  outStr,
			})
			return
		}

		// After install, check if binary is now found
		found := false
		foundPath := ""
		if binaryName != "" {
			if p, lookErr := exec.LookPath(binaryName); lookErr == nil {
				found = true
				foundPath = p
			}
		}

		jsonResponse(w, map[string]any{
			"success": true,
			"output":  outStr,
			"found":   found,
			"path":    foundPath,
		})
	}
}

func handleRegisterAgent(dispatcher *agent.Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			BinaryPath   string `json:"binaryPath"`
			DefaultModel string `json:"defaultModel"`
			EnvKey       string `json:"envKey"`
			EnvValue     string `json:"envValue"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.BinaryPath) == "" {
			jsonResponse(w, map[string]any{"success": false, "error": "id and binaryPath are required"})
			return
		}
		dispatcher.RegisterCustomAgent(req.ID, req.Name, req.BinaryPath, req.DefaultModel, req.EnvKey, req.EnvValue)
		jsonResponse(w, map[string]any{"success": true})
	}
}

func handleAgentAuthAction() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action string `json:"action"` // "logout", "status"
			Agent  string `json:"agent"`  // "claude", "codex", etc.
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err, http.StatusBadRequest)
			return
		}

		agent := strings.ToLower(strings.TrimSpace(req.Agent))
		action := strings.ToLower(strings.TrimSpace(req.Action))

		if action == "logout" {
			if strings.Contains(agent, "claude") {
				if bin, err := exec.LookPath("claude"); err == nil {
					cmd := exec.Command(bin, "auth", "logout")
					out, err := cmd.CombinedOutput()
					if err != nil {
						jsonResponse(w, map[string]any{"success": false, "error": string(out)})
						return
					}
					jsonResponse(w, map[string]any{"success": true, "message": "Logged out successfully"})
					return
				}
			}
			jsonResponse(w, map[string]any{"success": false, "error": "Logout not supported for this agent"})
			return
		}

		jsonResponse(w, map[string]any{"success": false, "error": "Unknown action"})
	}
}

func handleGetAntigravityProjects(w http.ResponseWriter, r *http.Request) {
	groups, err := session.LoadAntigravityProjects()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	jsonResponse(w, groups)
}

func handleGetAntigravityMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	msgs, err := session.ReadAntigravityTranscript(id)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []session.Message{}
	}
	jsonResponse(w, msgs)
}

func handleGetMediaFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	if !PathAllowed(path) {
		forbidPath(w)
		return
	}
	// Đảm bảo phục vụ file ảnh hợp lệ
	http.ServeFile(w, r, path)
}

func handleUploadFile(w http.ResponseWriter, r *http.Request) {
	// Giới hạn upload tối đa 32MB
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpError(w, fmt.Errorf("file too large or invalid multipart form: %w", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpError(w, fmt.Errorf("missing file parameter: %w", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	home, err := os.UserHomeDir()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	uploadsDir := filepath.Join(home, ".agent-hub", "uploads")
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		httpError(w, fmt.Errorf("create uploads directory: %w", err), http.StatusInternalServerError)
		return
	}

	ext := filepath.Ext(header.Filename)
	safeName := fmt.Sprintf("upload_%d%s", time.Now().UnixNano(), ext)
	targetPath := filepath.Join(uploadsDir, safeName)

	dst, err := os.Create(targetPath)
	if err != nil {
		httpError(w, fmt.Errorf("create target file: %w", err), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		httpError(w, fmt.Errorf("write uploaded file: %w", err), http.StatusInternalServerError)
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	jsonResponse(w, map[string]any{
		"uri":       targetPath,
		"mime_type": contentType,
		"url":       "/api/media?path=" + targetPath,
		"filename":  header.Filename,
	})
}

// --- WebSocket Handler ---

// PromptRequest is the JSON message sent by the client via WebSocket.
type PromptRequest struct {
	Type      string              `json:"type"`
	SessionID string              `json:"session_id"`
	Content   string              `json:"content"`
	Agent     string              `json:"agent"`
	Model     string              `json:"model"`
	Effort    string              `json:"effort"`
	Workspace string              `json:"workspace"`
	Media     []session.MediaItem `json:"media"`
}

func handleWebSocket(hub *Hub, sm *session.Manager, dispatcher *agent.Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[ws] Upgrade error: %v", err)
			return
		}

		client := &Client{
			hub:  hub,
			conn: conn,
			send: make(chan []byte, 256),
		}

		hub.register <- client
		go client.writePump()
		client.readPump(func(c *Client, message []byte) {
			var req PromptRequest
			if err := json.Unmarshal(message, &req); err != nil {
				sendWSError(c, "Invalid JSON message")
				return
			}

			switch req.Type {
			case "prompt":
				handlePrompt(c, hub, sm, dispatcher, req)
			case "subscribe":
				c.sessionID = req.SessionID
			case "approve":
				// User bấm Approve: gửi "y\n" hoặc lệnh vào stdin
				_ = dispatcher.SendInput(req.SessionID, "y\n")
				sendWSEvent(hub, req.SessionID, map[string]any{
					"type":     "approval_resolved",
					"approved": true,
				})
			case "reject":
				// User bấm Reject: gửi "n\n"
				_ = dispatcher.SendInput(req.SessionID, "n\n")
				sendWSEvent(hub, req.SessionID, map[string]any{
					"type":     "approval_resolved",
					"approved": false,
				})
			case "send_input":
				_ = dispatcher.SendInput(req.SessionID, req.Content+"\n")
			case "cancel":
				dispatcher.KillSession(req.SessionID)
				sendWSEvent(hub, req.SessionID, map[string]any{
					"type": "task_finished",
				})
			default:
				sendWSError(c, "Unknown message type: "+req.Type)
			}
		})
	}
}

func handlePrompt(client *Client, hub *Hub, sm *session.Manager, dispatcher *agent.Dispatcher, req PromptRequest) {
	// Update client session
	client.sessionID = req.SessionID

	// Ensure session exists
	if req.SessionID == "" {
		s, err := sm.CreateSession("New Chat", req.Workspace)
		if err != nil {
			sendWSError(client, "Failed to create session: "+err.Error())
			return
		}
		req.SessionID = s.ID
		client.sessionID = s.ID
		// Notify client of new session ID
		sendWSEvent(hub, req.SessionID, map[string]any{
			"type":       "session_created",
			"session_id": s.ID,
		})
	}

	// Save user message
	sm.SaveMessage(&session.Message{
		SessionID: req.SessionID,
		Role:      "user",
		Content:   req.Content,
		Media:     req.Media,
	})

	// Get context for injection
	contextStr, _ := sm.GetContext(req.SessionID, 20)
	_ = contextStr // Will be used when context adapter is wired

	// Default agent
	agentName := req.Agent
	if agentName == "" {
		agentName = "agy"
	}

	// Format prompt with media attachment info if present
	effectivePrompt := req.Content
	if len(req.Media) > 0 {
		var mediaNote strings.Builder
		mediaNote.WriteString(effectivePrompt)
		mediaNote.WriteString("\n\nAttached media:")
		for _, m := range req.Media {
			mediaNote.WriteString("\n- ")
			mediaNote.WriteString(m.URI)
		}
		effectivePrompt = mediaNote.String()
	}

	// Dispatch to agent with reasoning effort
	ctx := context.Background()
	events, err := dispatcher.Dispatch(ctx, req.SessionID, agentName, req.Model, req.Effort, effectivePrompt, req.Workspace)
	if err != nil {
		sendWSError(client, "Dispatch error: "+err.Error())
		return
	}

	// Stream events to client
	go func() {
		var fullContent string
		for evt := range events {
			if evt.Type == "session_created" && evt.SessionID != "" {
				client.sessionID = evt.SessionID
				req.SessionID = evt.SessionID
			}
			if evt.Type == "token" {
				fullContent += evt.Content
			}
			sendWSEvent(hub, req.SessionID, evt)
		}

		// Đảm bảo client luôn nhận task_finished khi kết thúc
		sendWSEvent(hub, req.SessionID, map[string]any{
			"type": "task_finished",
		})

		// Save assistant message
		if fullContent != "" {
			sm.SaveMessage(&session.Message{
				SessionID: req.SessionID,
				Role:      "assistant",
				Content:   fullContent,
				Agent:     agentName,
				Model:     req.Model,
			})
		}
	}()
}

// --- Helpers ---

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func httpError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func sendWSEvent(hub *Hub, sessionID string, event any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	hub.BroadcastToSession(sessionID, data)
}

func sendWSError(client *Client, message string) {
	data, _ := json.Marshal(map[string]string{"type": "error", "message": message})
	client.send <- data
}

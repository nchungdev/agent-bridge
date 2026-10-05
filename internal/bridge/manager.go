package bridge

import (
	"crypto/sha1"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Workspace represents a registered folder
type Workspace struct {
	ID                    string    `json:"id"`
	Path                  string    `json:"path"`
	Name                  string    `json:"name"`
	ActiveBridgeSessionID string    `json:"active_bridge_session_id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// ToolBinding represents a native session of a tool bound to a task
type ToolBinding struct {
	AgentName       string    `json:"agent"`
	NativeSessionID string    `json:"native_session_id"`
	Title           string    `json:"title,omitempty"`
	TurnCount       int       `json:"turn_count,omitempty"`
	LastSyncedHash  string    `json:"last_synced_hash,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// BridgeSession is a universal task session (Task)
type BridgeSession struct {
	ID                 string            `json:"id"`
	WorkspaceID        string            `json:"workspace_id"`
	Workspace          string            `json:"workspace,omitempty"`
	Title              string            `json:"title"`
	Status             string            `json:"status"` // active, paused, completed
	CurrentAgent       string            `json:"current_agent"`
	LastHandoffSummary string            `json:"last_handoff_summary"`
	Bindings           map[string]string `json:"bindings"` // agent -> native_session_id
	Tools              []ToolBinding     `json:"tools"`    // level 3 native sessions per tool
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

// AgentBinding maps universal session to native agent session ID
type AgentBinding struct {
	ID              int64     `json:"id"`
	BridgeSessionID string    `json:"bridge_session_id"`
	AgentName       string    `json:"agent_name"`
	NativeSessionID string    `json:"native_session_id"`
	SessionFilePath string    `json:"session_file_path"`
	LastSyncedHash  string    `json:"last_synced_hash"`
	IsCurrent       bool      `json:"is_current"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// HandoffCheckpoint records a context switch event
type HandoffCheckpoint struct {
	ID              int64     `json:"id"`
	BridgeSessionID string    `json:"bridge_session_id"`
	FromAgent       string    `json:"from_agent"`
	ToAgent         string    `json:"to_agent"`
	FromNativeID    string    `json:"from_native_id"`
	ToNativeID      string    `json:"to_native_id"`
	TriggerReason   string    `json:"trigger_reason"`
	TaskGoal        string    `json:"task_goal"`
	GitDiffSummary  string    `json:"git_diff_summary"`
	ContextSnapshot string    `json:"context_snapshot"`
	CreatedAt       time.Time `json:"created_at"`
}

// Manager orchestrates workspaces, sessions, and context handoffs
type Manager struct {
	db *sql.DB
}

func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// EnsureWorkspace gets or creates a workspace for the given path
func (m *Manager) EnsureWorkspace(path string) (*Workspace, error) {
	cleanPath := filepath.Clean(path)
	name := filepath.Base(cleanPath)
	sum := sha1.Sum([]byte(cleanPath))
	id := fmt.Sprintf("ws_%x", sum[:6])

	var ws Workspace
	err := m.db.QueryRow(`
		SELECT id, path, name, COALESCE(active_bridge_session_id, ''), created_at, updated_at
		FROM workspaces WHERE path = ?
	`, cleanPath).Scan(&ws.ID, &ws.Path, &ws.Name, &ws.ActiveBridgeSessionID, &ws.CreatedAt, &ws.UpdatedAt)

	if err == nil {
		return &ws, nil
	}

	now := time.Now()
	_, err = m.db.Exec(`
		INSERT INTO workspaces (id, path, name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, cleanPath, name, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert workspace: %w", err)
	}

	return &Workspace{
		ID:        id,
		Path:      cleanPath,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// ListWorkspaces returns all workspaces
func (m *Manager) ListWorkspaces() ([]Workspace, error) {
	rows, err := m.db.Query(`
		SELECT id, path, name, COALESCE(active_bridge_session_id, ''), created_at, updated_at
		FROM workspaces ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Workspace
	for rows.Next() {
		var ws Workspace
		if err := rows.Scan(&ws.ID, &ws.Path, &ws.Name, &ws.ActiveBridgeSessionID, &ws.CreatedAt, &ws.UpdatedAt); err == nil {
			list = append(list, ws)
		}
	}
	return list, nil
}

// CreateOrUpdateWorkspace creates a new workspace with custom name, optionally creating directory
func (m *Manager) CreateOrUpdateWorkspace(path, customName string, createDirIfMissing bool) (*Workspace, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	cleanPath := filepath.Clean(path)
	if createDirIfMissing {
		_ = os.MkdirAll(cleanPath, 0755)
	}

	name := strings.TrimSpace(customName)
	if name == "" {
		name = filepath.Base(cleanPath)
	}
	sum := sha1.Sum([]byte(cleanPath))
	id := fmt.Sprintf("ws_%x", sum[:6])
	now := time.Now()

	var existingID string
	err := m.db.QueryRow(`SELECT id FROM workspaces WHERE path = ?`, cleanPath).Scan(&existingID)
	if err == nil {
		_, _ = m.db.Exec(`UPDATE workspaces SET name = ?, updated_at = ? WHERE id = ?`, name, now, existingID)
		return &Workspace{
			ID:        existingID,
			Path:      cleanPath,
			Name:      name,
			UpdatedAt: now,
		}, nil
	}

	_, err = m.db.Exec(`
		INSERT INTO workspaces (id, path, name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, cleanPath, name, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert workspace: %w", err)
	}

	return &Workspace{
		ID:        id,
		Path:      cleanPath,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// UpdateWorkspaceName updates the display name of a workspace
func (m *Manager) UpdateWorkspaceName(workspaceID, newName string) error {
	trimmed := strings.TrimSpace(newName)
	if trimmed == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}
	_, err := m.db.Exec(`UPDATE workspaces SET name = ?, updated_at = ? WHERE id = ?`, trimmed, time.Now(), workspaceID)
	return err
}

// DeleteWorkspace unregisters a workspace and its sessions from agent bridge
func (m *Manager) DeleteWorkspace(workspaceID string) error {
	_, _ = m.db.Exec(`DELETE FROM agent_bindings WHERE bridge_session_id IN (SELECT id FROM bridge_sessions WHERE workspace_id = ?)`, workspaceID)
	_, _ = m.db.Exec(`DELETE FROM bridge_sessions WHERE workspace_id = ?`, workspaceID)
	_, err := m.db.Exec(`DELETE FROM workspaces WHERE id = ?`, workspaceID)
	return err
}

// BrowseDirectories returns subdirectories in the given path (or $HOME if empty)
func (m *Manager) BrowseDirectories(parentPath string) (string, []string, error) {
	if parentPath == "" || parentPath == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", nil, err
		}
		parentPath = home
	} else if strings.HasPrefix(parentPath, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			parentPath = filepath.Join(home, strings.TrimPrefix(parentPath, "~"))
		}
	}

	cleanPath := filepath.Clean(parentPath)
	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		return cleanPath, nil, err
	}

	var list []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__" {
			continue
		}
		list = append(list, name)
	}
	sort.Strings(list)
	return cleanPath, list, nil
}

// GetOrCreateSession gets or creates a universal session for a workspace
func (m *Manager) CreateSession(workspaceID, title, initialAgent string) (*BridgeSession, error) {
	sessID := fmt.Sprintf("sess_%d", time.Now().Unix())
	if initialAgent == "" {
		initialAgent = "agy"
	}
	now := time.Now()

	_, err := m.db.Exec(`
		INSERT INTO bridge_sessions (id, workspace_id, title, status, current_agent, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?)
	`, sessID, workspaceID, title, initialAgent, now, now)
	if err != nil {
		return nil, fmt.Errorf("create bridge session: %w", err)
	}

	// Update workspace active session
	_, _ = m.db.Exec(`UPDATE workspaces SET active_bridge_session_id = ?, updated_at = ? WHERE id = ?`, sessID, now, workspaceID)

	// Automatically register initial agent binding so it stays bound to this task
	_, _ = m.db.Exec(`
		INSERT INTO agent_bindings (bridge_session_id, agent_name, native_session_id, updated_at)
		VALUES (?, ?, '', ?)
	`, sessID, initialAgent, now)

	bindings := make(map[string]string)
	bindings[initialAgent] = ""
	return &BridgeSession{
		ID:           sessID,
		WorkspaceID:  workspaceID,
		Title:        title,
		Status:       "active",
		CurrentAgent: initialAgent,
		Bindings:     bindings,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// BindAgent links a native agent session ID to the universal bridge session
func (m *Manager) BindAgent(bridgeSessionID, agentName, nativeSessionID string) error {
	now := time.Now()
	_, err := m.db.Exec(`
		INSERT INTO agent_bindings (bridge_session_id, agent_name, native_session_id, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(bridge_session_id, agent_name) DO UPDATE SET
			native_session_id = CASE WHEN excluded.native_session_id != '' THEN excluded.native_session_id ELSE agent_bindings.native_session_id END,
			updated_at = excluded.updated_at
	`, bridgeSessionID, agentName, nativeSessionID, now)
	if err != nil {
		return err
	}
	_, _ = m.db.Exec(`UPDATE bridge_sessions SET updated_at = ? WHERE id = ?`, now, bridgeSessionID)
	return nil
}

// GetSessionBindingsDetailed returns the list of ToolBinding and map for a bridge session
func (m *Manager) GetSessionBindingsDetailed(bridgeSessionID, wsPath string) ([]ToolBinding, map[string]string, error) {
	rows, err := m.db.Query(`
		SELECT agent_name, native_session_id, COALESCE(last_synced_hash, ''), updated_at
		FROM agent_bindings 
		WHERE bridge_session_id = ? 
		ORDER BY CASE agent_name 
			WHEN 'agy' THEN 1 
			WHEN 'claude' THEN 2 
			WHEN 'codex' THEN 3 
			ELSE 4 
		END, agent_name ASC
	`, bridgeSessionID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var tools []ToolBinding
	bindingsMap := make(map[string]string)
	for rows.Next() {
		var tb ToolBinding
		if err := rows.Scan(&tb.AgentName, &tb.NativeSessionID, &tb.LastSyncedHash, &tb.UpdatedAt); err == nil {
			bindingsMap[tb.AgentName] = tb.NativeSessionID
			// Enrich with native session info if available
			if wsPath != "" && tb.NativeSessionID != "" {
				if nat := m.GetNativeSession(tb.AgentName, tb.NativeSessionID, wsPath); nat != nil {
					tb.Title = nat.Title
					tb.TurnCount = nat.TurnCount
				}
			}
			tools = append(tools, tb)
		}
	}
	return tools, bindingsMap, nil
}

// GetWorkspaceSessions returns all sessions for a workspace, including their agent bindings and level 3 tools
func (m *Manager) GetWorkspaceSessions(workspaceID, wsPath string) ([]BridgeSession, error) {
	rows, err := m.db.Query(`
		SELECT id, workspace_id, title, status, current_agent, COALESCE(last_handoff_summary, ''), created_at, updated_at
		FROM bridge_sessions WHERE workspace_id = ? ORDER BY updated_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}

	var list []BridgeSession
	for rows.Next() {
		var s BridgeSession
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.Title, &s.Status, &s.CurrentAgent, &s.LastHandoffSummary, &s.CreatedAt, &s.UpdatedAt); err == nil {
			s.Workspace = wsPath
			s.Bindings = make(map[string]string)
			s.Tools = []ToolBinding{}
			list = append(list, s)
		}
	}
	_ = rows.Close()

	for i := range list {
		tools, bMap, _ := m.GetSessionBindingsDetailed(list[i].ID, wsPath)
		if bMap == nil {
			bMap = make(map[string]string)
		}
		if tools == nil {
			tools = []ToolBinding{}
		}
		list[i].Bindings = bMap
		list[i].Tools = tools
	}
	return list, nil
}

// GetWorkspaceSessionsByPath resolves workspace path then returns its sessions
func (m *Manager) GetWorkspaceSessionsByPath(workspacePath string) ([]BridgeSession, error) {
	ws, err := m.EnsureWorkspace(workspacePath)
	if err != nil {
		return nil, err
	}
	return m.GetWorkspaceSessions(ws.ID, workspacePath)
}

// SyncHandoff checks if targetAgent needs context from previous work in this task.
// Returns (synced: bool, message: string, err: error)
// It is smart: if targetAgent is already up-to-date or content is identical, it skips reading!
func (m *Manager) SyncHandoff(workspacePath, taskID, targetAgent, sourceAgent string) (bool, string, error) {
	gitSummary, modFiles := m.CaptureWorkspaceState(workspacePath)

	// Calculate deterministic hash of current workspace state
	hasher := sha1.New()
	hasher.Write([]byte(gitSummary))
	for _, f := range modFiles {
		hasher.Write([]byte(f))
	}
	currentHash := fmt.Sprintf("%x", hasher.Sum(nil))[:16]

	// 1. Get current binding of targetAgent
	var lastSyncedHash string
	err := m.db.QueryRow(`
		SELECT COALESCE(last_synced_hash, '')
		FROM agent_bindings WHERE bridge_session_id = ? AND agent_name = ?
	`, taskID, targetAgent).Scan(&lastSyncedHash)
	if err != nil && err != sql.ErrNoRows {
		return false, "", err
	}

	// If targetAgent is same as sourceAgent and already synced, skip
	if targetAgent == sourceAgent && lastSyncedHash == currentHash && currentHash != "" {
		return false, "Nội dung không đổi, tool đã ở phiên làm việc mới nhất", nil
	}

	// If hash is identical, skip (avoid duplicate reading!)
	if lastSyncedHash == currentHash && currentHash != "" {
		return false, "Nội dung giống trong session rồi, không cần tải lại", nil
	}

	// 2. Perform handoff
	taskGoal := ""
	_ = m.db.QueryRow(`SELECT title FROM bridge_sessions WHERE id = ?`, taskID).Scan(&taskGoal)

	triggerReason := fmt.Sprintf("Chuyển từ %s sang %s", sourceAgent, targetAgent)
	extraContext := ""
	if sourceAgent != "" && sourceAgent != targetAgent {
		if src := m.GetNativeSession(sourceAgent, "", workspacePath); src != nil {
			extraContext = fmt.Sprintf("### Lời nhắn từ %s\nPhiên trước đó vừa kết thúc tại turn %d.", sourceAgent, src.TurnCount)
		}
	}

	_, err = m.ExecuteHandoff(workspacePath, taskID, sourceAgent, targetAgent, taskGoal, triggerReason, extraContext)
	if err != nil {
		return false, "", fmt.Errorf("execute handoff: %w", err)
	}

	// 3. Update or insert targetAgent's last_synced_hash
	now := time.Now()
	_, _ = m.db.Exec(`
		INSERT INTO agent_bindings (bridge_session_id, agent_name, native_session_id, last_synced_hash, updated_at)
		VALUES (?, ?, '', ?, ?)
		ON CONFLICT(bridge_session_id, agent_name) DO UPDATE SET
			last_synced_hash = excluded.last_synced_hash,
			updated_at = excluded.updated_at
	`, taskID, targetAgent, currentHash, now)

	msg := fmt.Sprintf("Đã đồng bộ handoff mới từ %s (%d files thay đổi)", sourceAgent, len(modFiles))
	return true, msg, nil
}

// UpdateSessionTitle updates the title of a bridge session
func (m *Manager) UpdateSessionTitle(bridgeSessionID, title string) error {
	_, err := m.db.Exec(`UPDATE bridge_sessions SET title = ?, updated_at = ? WHERE id = ?`, title, time.Now(), bridgeSessionID)
	return err
}

// DeleteSession removes a bridge session and its bindings
func (m *Manager) DeleteSession(bridgeSessionID string) error {
	_, _ = m.db.Exec(`DELETE FROM agent_bindings WHERE bridge_session_id = ?`, bridgeSessionID)
	_, err := m.db.Exec(`DELETE FROM bridge_sessions WHERE id = ?`, bridgeSessionID)
	return err
}

// CaptureWorkspaceState extracts git diff and modified files
func (m *Manager) CaptureWorkspaceState(workspacePath string) (gitSummary string, modifiedFiles []string) {
	if _, err := os.Stat(filepath.Join(workspacePath, ".git")); err != nil {
		return "Not a git repository", nil
	}

	cmdStatus := exec.Command("git", "status", "-s")
	cmdStatus.Dir = workspacePath
	if out, err := cmdStatus.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, l := range lines {
			parts := strings.Fields(l)
			if len(parts) >= 2 {
				modifiedFiles = append(modifiedFiles, parts[1])
			}
		}
	}

	cmdDiff := exec.Command("git", "diff", "--stat")
	cmdDiff.Dir = workspacePath
	if out, err := cmdDiff.Output(); err == nil {
		gitSummary = strings.TrimSpace(string(out))
	}
	if gitSummary == "" && len(modifiedFiles) > 0 {
		gitSummary = strings.Join(modifiedFiles, ", ")
	}
	return gitSummary, modifiedFiles
}

// ExecuteHandoff captures context, saves checkpoint, and writes local .agent/handoff.md
func (m *Manager) ExecuteHandoff(workspacePath, sessionID, fromAgent, toAgent, taskGoal, triggerReason, extraContext string) (*HandoffCheckpoint, error) {
	gitSummary, modFiles := m.CaptureWorkspaceState(workspacePath)

	now := time.Now()
	// Build Markdown snapshot
	var snap strings.Builder
	snap.WriteString("## AGENT BRIDGE CONTEXT HANDOFF\n\n")
	snap.WriteString(fmt.Sprintf("- **Workspace**: `%s`\n", workspacePath))
	snap.WriteString(fmt.Sprintf("- **Handoff Time**: %s\n", now.Format(time.RFC3339)))
	snap.WriteString(fmt.Sprintf("- **From Engine**: `%s`\n", fromAgent))
	snap.WriteString(fmt.Sprintf("- **To Engine**: `%s`\n", toAgent))
	snap.WriteString(fmt.Sprintf("- **Trigger Reason**: %s\n\n", triggerReason))

	if taskGoal != "" {
		snap.WriteString(fmt.Sprintf("### Current Task Goal\n%s\n\n", taskGoal))
	}
	if len(modFiles) > 0 {
		snap.WriteString("### Modified Files\n")
		for _, f := range modFiles {
			snap.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
		snap.WriteString("\n")
	}
	if gitSummary != "" && gitSummary != "Not a git repository" {
		snap.WriteString("### Git Changes Summary\n```text\n")
		snap.WriteString(gitSummary)
		snap.WriteString("\n```\n\n")
	}
	if extraContext != "" {
		snap.WriteString(fmt.Sprintf("### Context Notes & Next Steps\n%s\n\n", extraContext))
	}
	snap.WriteString("### Instructions for Receiving Agent\n")
	snap.WriteString("Resume the task seamlessly from the above state. Inspect the modified files first before proposing next actions.\n")

	snapshotStr := snap.String()

	// Write to .agent/handoff.md in workspace
	agentDir := filepath.Join(workspacePath, ".agent")
	_ = os.MkdirAll(agentDir, 0755)
	_ = os.WriteFile(filepath.Join(agentDir, "handoff.md"), []byte(snapshotStr), 0644)

	// Save Checkpoint to DB
	res, err := m.db.Exec(`
		INSERT INTO handoff_checkpoints (
			bridge_session_id, from_agent, to_agent, trigger_reason,
			task_goal, git_diff_summary, context_snapshot, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, fromAgent, toAgent, triggerReason, taskGoal, gitSummary, snapshotStr, now)
	if err != nil {
		return nil, fmt.Errorf("insert checkpoint: %w", err)
	}

	chkID, _ := res.LastInsertId()

	// Update session status and current agent
	_, _ = m.db.Exec(`
		UPDATE bridge_sessions
		SET current_agent = ?, last_handoff_summary = ?, updated_at = ?
		WHERE id = ?
	`, toAgent, taskGoal, now, sessionID)

	return &HandoffCheckpoint{
		ID:              chkID,
		BridgeSessionID: sessionID,
		FromAgent:       fromAgent,
		ToAgent:         toAgent,
		TriggerReason:   triggerReason,
		TaskGoal:        taskGoal,
		GitDiffSummary:  gitSummary,
		ContextSnapshot: snapshotStr,
		CreatedAt:       now,
	}, nil
}

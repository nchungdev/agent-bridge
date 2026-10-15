package bridge

import (
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

// Subtask is a breakdown item of a task
type Subtask struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// BridgeSession is a universal task session (Task)
type BridgeSession struct {
	ID                 string            `json:"id"`
	WorkspaceID        string            `json:"workspace_id"`
	Workspace          string            `json:"workspace,omitempty"`
	Title              string            `json:"title"`
	Status             string            `json:"status"` // active, paused, completed
	KanbanStatus       string            `json:"kanban_status"` // todo, in_progress, review, done
	Subtasks           []Subtask         `json:"subtasks"`
	RemoteURL          string            `json:"remote_url,omitempty"`
	OrderIdx           int               `json:"order_idx"`
	ResultSummary      string            `json:"result_summary,omitempty"`
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
	db           *sql.DB
	discoverMu   sync.Mutex
	lastDiscover map[string]time.Time
}

func NewManager(db *sql.DB) *Manager {
	return &Manager{
		db:           db,
		lastDiscover: make(map[string]time.Time),
	}
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

// IsProtectedWorkspacePath checks if the given workspace path is a critical system directory
// or user home folder (e.g. /, /root, /home, /Users, /Users/username, /home/username).
func IsProtectedWorkspacePath(p string) bool {
	if strings.TrimSpace(p) == "" {
		return true
	}
	clean := filepath.Clean(strings.TrimSpace(p))
	if clean == "/" || clean == "." || clean == "~" {
		return true
	}

	// Host home directory
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if clean == filepath.Clean(home) {
			return true
		}
	}

	// Exact system roots
	systemRoots := []string{
		"/root", "/home", "/Users", "/bin", "/sbin", "/usr", "/etc", "/var", "/dev", "/proc", "/sys", "/opt", "/tmp",
	}
	for _, sys := range systemRoots {
		if clean == sys {
			return true
		}
	}

	// Direct user profile / home directory: /home/<user> or /Users/<user>
	parent := filepath.Dir(clean)
	if parent == "/home" || parent == "/Users" {
		return true
	}

	// Windows system directories if running on Windows
	vol := filepath.VolumeName(clean)
	withoutVol := strings.TrimPrefix(clean, vol)
	if withoutVol == `\` || withoutVol == "" {
		return true
	}
	winParent := filepath.Dir(clean)
	if strings.EqualFold(winParent, filepath.Join(vol, `\Users`)) || strings.EqualFold(clean, filepath.Join(vol, `\Windows`)) {
		return true
	}

	return false
}

// DeleteWorkspace unregisters a workspace and its sessions from agent bridge
func (m *Manager) DeleteWorkspace(workspaceID string) error {
	var path string
	err := m.db.QueryRow(`SELECT path FROM workspaces WHERE id = ?`, workspaceID).Scan(&path)
	if err == nil && IsProtectedWorkspacePath(path) {
		return fmt.Errorf("thư mục %q thuộc danh mục hệ thống/tài khoản được bảo vệ, không thể xoá", path)
	}

	_, _ = m.db.Exec(`DELETE FROM agent_bindings WHERE bridge_session_id IN (SELECT id FROM bridge_sessions WHERE workspace_id = ?)`, workspaceID)
	_, _ = m.db.Exec(`DELETE FROM bridge_sessions WHERE workspace_id = ?`, workspaceID)
	_, err = m.db.Exec(`DELETE FROM workspaces WHERE id = ?`, workspaceID)
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

// CreateSession gets or creates a universal session for a workspace (backward compatibility)
func (m *Manager) CreateSession(workspaceID, title, initialAgent string) (*BridgeSession, error) {
	return m.CreateSessionWithKanban(workspaceID, title, initialAgent, "todo", nil)
}

// CreateSessionWithKanban creates a universal session with initial kanban status and subtasks
func (m *Manager) CreateSessionWithKanban(workspaceID, title, initialAgent, kanbanStatus string, subtasks []Subtask) (*BridgeSession, error) {
	sessID := fmt.Sprintf("sess_%d", time.Now().Unix())
	if initialAgent == "" {
		initialAgent = "agy"
	}
	if kanbanStatus == "" {
		kanbanStatus = "todo"
	}
	if subtasks == nil {
		subtasks = []Subtask{}
	}
	subtasksJSON, _ := json.Marshal(subtasks)
	now := time.Now()

	_, err := m.db.Exec(`
		INSERT INTO bridge_sessions (id, workspace_id, title, status, kanban_status, subtasks_json, current_agent, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?, ?, ?)
	`, sessID, workspaceID, title, kanbanStatus, string(subtasksJSON), initialAgent, now, now)
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
		KanbanStatus: kanbanStatus,
		Subtasks:     subtasks,
		CurrentAgent: initialAgent,
		Bindings:     bindings,
		Tools:        []ToolBinding{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// UpdateTaskKanban updates the kanban column status and order index of a task
func (m *Manager) UpdateTaskKanban(taskID, kanbanStatus string, orderIdx int) error {
	now := time.Now()
	legacyStatus := "active"
	if kanbanStatus == "done" {
		legacyStatus = "completed"
	}
	_, err := m.db.Exec(`
		UPDATE bridge_sessions 
		SET kanban_status = ?, order_idx = ?, status = ?, updated_at = ? 
		WHERE id = ?
	`, kanbanStatus, orderIdx, legacyStatus, now, taskID)
	return err
}

// UpdateTaskSubtasks updates the whole subtask list of a task
func (m *Manager) UpdateTaskSubtasks(taskID string, subtasks []Subtask) error {
	if subtasks == nil {
		subtasks = []Subtask{}
	}
	bytes, err := json.Marshal(subtasks)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = m.db.Exec(`UPDATE bridge_sessions SET subtasks_json = ?, updated_at = ? WHERE id = ?`, string(bytes), now, taskID)
	return err
}

// ToggleSubtask toggles the done state of a specific subtask within a task
func (m *Manager) ToggleSubtask(taskID, subtaskID string, done bool) error {
	var subtasksRaw string
	err := m.db.QueryRow(`SELECT COALESCE(subtasks_json, '[]') FROM bridge_sessions WHERE id = ?`, taskID).Scan(&subtasksRaw)
	if err != nil {
		return err
	}
	var subtasks []Subtask
	_ = json.Unmarshal([]byte(subtasksRaw), &subtasks)
	for i := range subtasks {
		if subtasks[i].ID == subtaskID {
			subtasks[i].Done = done
			break
		}
	}
	return m.UpdateTaskSubtasks(taskID, subtasks)
}

// AddSubtask appends a new subtask to a task
func (m *Manager) AddSubtask(taskID, title string) (*Subtask, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("subtask title cannot be empty")
	}
	var subtasksRaw string
	err := m.db.QueryRow(`SELECT COALESCE(subtasks_json, '[]') FROM bridge_sessions WHERE id = ?`, taskID).Scan(&subtasksRaw)
	if err != nil {
		return nil, err
	}
	var subtasks []Subtask
	_ = json.Unmarshal([]byte(subtasksRaw), &subtasks)
	newSubtask := Subtask{
		ID:        fmt.Sprintf("st_%d", time.Now().UnixNano()),
		Title:     title,
		Done:      false,
		CreatedAt: time.Now(),
	}
	subtasks = append(subtasks, newSubtask)
	if err := m.UpdateTaskSubtasks(taskID, subtasks); err != nil {
		return nil, err
	}
	return &newSubtask, nil
}

// DeleteSubtask removes a subtask from a task
func (m *Manager) DeleteSubtask(taskID, subtaskID string) error {
	var subtasksRaw string
	err := m.db.QueryRow(`SELECT COALESCE(subtasks_json, '[]') FROM bridge_sessions WHERE id = ?`, taskID).Scan(&subtasksRaw)
	if err != nil {
		return err
	}
	var subtasks []Subtask
	_ = json.Unmarshal([]byte(subtasksRaw), &subtasks)
	filtered := make([]Subtask, 0, len(subtasks))
	for _, st := range subtasks {
		if st.ID != subtaskID {
			filtered = append(filtered, st)
		}
	}
	return m.UpdateTaskSubtasks(taskID, filtered)
}

// UpdateTaskAgent updates the assigned agent for a task
func (m *Manager) UpdateTaskAgent(taskID, agent string) error {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return fmt.Errorf("agent cannot be empty")
	}
	now := time.Now()
	_, err := m.db.Exec(`UPDATE bridge_sessions SET current_agent = ?, updated_at = ? WHERE id = ?`, agent, now, taskID)
	return err
}

// UpdateTaskResult updates the result summary and optionally status
func (m *Manager) UpdateTaskResult(taskID, summary, kanbanStatus string) error {
	now := time.Now()
	if kanbanStatus == "" {
		kanbanStatus = "done"
	}
	legacyStatus := "active"
	if kanbanStatus == "done" {
		legacyStatus = "completed"
	}
	_, err := m.db.Exec(`
		UPDATE bridge_sessions 
		SET result_summary = ?, kanban_status = ?, status = ?, updated_at = ? 
		WHERE id = ?
	`, summary, kanbanStatus, legacyStatus, now, taskID)
	return err
}

// UpdateTaskRemoteURL updates the remote access URL for the task
func (m *Manager) UpdateTaskRemoteURL(taskID, remoteURL string) error {
	now := time.Now()
	_, err := m.db.Exec(`UPDATE bridge_sessions SET remote_url = ?, updated_at = ? WHERE id = ?`, remoteURL, now, taskID)
	return err
}

// CleanupTodoTasks purges all todo tasks for a given workspace
func (m *Manager) CleanupTodoTasks(workspacePath string) (int64, error) {
	ws, err := m.EnsureWorkspace(workspacePath)
	if err != nil {
		return 0, err
	}
	_, _ = m.db.Exec(`
		DELETE FROM agent_bindings 
		WHERE bridge_session_id IN (
			SELECT id FROM bridge_sessions WHERE workspace_id = ? AND kanban_status = 'todo'
		)
	`, ws.ID)
	res, err := m.db.Exec(`DELETE FROM bridge_sessions WHERE workspace_id = ? AND kanban_status = 'todo'`, ws.ID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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
	var rawTools []ToolBinding
	for rows.Next() {
		var tb ToolBinding
		if err := rows.Scan(&tb.AgentName, &tb.NativeSessionID, &tb.LastSyncedHash, &tb.UpdatedAt); err == nil {
			rawTools = append(rawTools, tb)
		}
	}
	_ = rows.Close()

	var tools []ToolBinding
	bindingsMap := make(map[string]string)
	for _, tb := range rawTools {
		bindingsMap[tb.AgentName] = tb.NativeSessionID
		// Fast metadata lookup without heavy transcript parsing
		if wsPath != "" && tb.NativeSessionID != "" {
			if meta := m.GetNativeMeta(tb.AgentName, tb.NativeSessionID, wsPath); meta != nil {
				tb.Title = meta.Title
				tb.TurnCount = meta.TurnCount
			}
		}
		tools = append(tools, tb)
	}
	return tools, bindingsMap, nil
}

// AutoBindLatestUnboundSession searches for an unbound native session of agent in workspace
// and binds it to bridgeSessionID, syncing metadata.
func (m *Manager) AutoBindLatestUnboundSession(bridgeSessionID, agentName, wsPath string) string {
	if bridgeSessionID == "" || agentName == "" || wsPath == "" {
		return ""
	}

	var taskTitle string
	var taskCreated time.Time
	err := m.db.QueryRow(`SELECT title, created_at FROM bridge_sessions WHERE id = ?`, bridgeSessionID).Scan(&taskTitle, &taskCreated)
	if err != nil {
		return ""
	}

	boundIDs := make(map[string]bool)
	rows, err := m.db.Query(`SELECT native_session_id FROM agent_bindings WHERE agent_name = ? AND native_session_id != ''`, agentName)
	if err == nil {
		for rows.Next() {
			var bid string
			if rows.Scan(&bid) == nil && bid != "" {
				boundIDs[bid] = true
			}
		}
		_ = rows.Close()
	}

	var nativeSessions []NativeSession
	base := BaseAgent(agentName)
	switch base {
	case "agy":
		nativeSessions = listAgySessions(wsPath, 30)
	case "claude":
		nativeSessions = listClaudeSessions(wsPath, 30)
	case "codex":
		nativeSessions = listCodexSessions(wsPath, 30)
	}

	cleanTitle := strings.TrimSpace(taskTitle)
	isDefaultTitle := cleanTitle == "" || cleanTitle == "New Task" || cleanTitle == "Phiên làm việc mới"

	var matchedID string
	// Pass 1: Try exact title match for non-default titles
	if !isDefaultTitle {
		for _, s := range nativeSessions {
			if !boundIDs[s.ID] && strings.EqualFold(strings.TrimSpace(s.Title), cleanTitle) {
				matchedID = s.ID
				break
			}
		}
	}

	// Pass 2: Pick the most recent unbound session
	if matchedID == "" {
		for _, s := range nativeSessions {
			if !boundIDs[s.ID] {
				matchedID = s.ID
				break
			}
		}
	}

	if matchedID != "" {
		_ = m.BindAgent(bridgeSessionID, agentName, matchedID)
		if base == "agy" {
			_ = SyncAgyConversationMeta(matchedID, taskTitle, wsPath)
		}
		return matchedID
	}

	return ""
}

// AutoDiscoverExternalSessions scans native session stores for agy, claude, codex
// and creates bridge_session Tasks for any unbound conversations created externally (e.g. in IDEs or CLIs).
func (m *Manager) AutoDiscoverExternalSessions(workspaceID, wsPath string) error {
	if workspaceID == "" || wsPath == "" {
		return nil
	}

	m.discoverMu.Lock()
	if m.lastDiscover == nil {
		m.lastDiscover = make(map[string]time.Time)
	}
	last, ok := m.lastDiscover[workspaceID]
	if ok && time.Since(last) < 3*time.Second {
		m.discoverMu.Unlock()
		return nil
	}
	m.lastDiscover[workspaceID] = time.Now()
	m.discoverMu.Unlock()

	boundIDs := make(map[string]bool)
	rows, err := m.db.Query(`SELECT native_session_id FROM agent_bindings WHERE native_session_id != ''`)
	if err == nil {
		for rows.Next() {
			var bid string
			if rows.Scan(&bid) == nil && bid != "" {
				boundIDs[bid] = true
			}
		}
		_ = rows.Close()
	}

	type candSess struct {
		agent string
		sess  NativeSession
	}
	var candidates []candSess

	for _, s := range listAgySessions(wsPath, 50) {
		if !boundIDs[s.ID] {
			candidates = append(candidates, candSess{agent: "agy", sess: s})
		}
	}
	for _, s := range listClaudeSessions(wsPath, 50) {
		if !boundIDs[s.ID] {
			candidates = append(candidates, candSess{agent: "claude", sess: s})
		}
	}
	for _, s := range listCodexSessions(wsPath, 50) {
		if !boundIDs[s.ID] {
			candidates = append(candidates, candSess{agent: "codex", sess: s})
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Sort candidates by UpdatedAt ascending so older conversations are inserted first
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].sess.UpdatedAt.Before(candidates[j].sess.UpdatedAt)
	})

	for _, c := range candidates {
		if boundIDs[c.sess.ID] {
			continue
		}

		title := strings.TrimSpace(c.sess.Title)
		if title == "" {
			if len(c.sess.Turns) > 0 {
				title = clip(firstUser(c.sess.Turns), 70)
			}
			if title == "" {
				title = "Phiên làm việc mới"
			}
		}

		createdAt := c.sess.UpdatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		suffix := c.sess.ID
		if len(suffix) > 12 {
			suffix = suffix[:12]
		}
		taskID := fmt.Sprintf("sess_%d_%s", createdAt.Unix(), suffix)

		_, err := m.db.Exec(`
			INSERT OR IGNORE INTO bridge_sessions (id, workspace_id, title, status, kanban_status, subtasks_json, current_agent, created_at, updated_at)
			VALUES (?, ?, ?, 'active', 'in_progress', '[]', ?, ?, ?)
		`, taskID, workspaceID, title, c.agent, createdAt, createdAt)
		if err != nil {
			continue
		}

		_, err = m.db.Exec(`
			INSERT OR IGNORE INTO agent_bindings (bridge_session_id, agent_name, native_session_id, is_current, updated_at)
			VALUES (?, ?, ?, 1, ?)
		`, taskID, c.agent, c.sess.ID, createdAt)
		if err != nil {
			continue
		}

		boundIDs[c.sess.ID] = true

		if c.agent == "agy" {
			_ = SyncAgyConversationMeta(c.sess.ID, title, wsPath)
		}
	}

	return nil
}

// GetWorkspaceSessions returns all sessions for a workspace, including their agent bindings and level 3 tools
func (m *Manager) GetWorkspaceSessions(workspaceID, wsPath string) ([]BridgeSession, error) {
	if wsPath == "" && workspaceID != "" {
		_ = m.db.QueryRow(`SELECT path FROM workspaces WHERE id = ?`, workspaceID).Scan(&wsPath)
	}

	// Auto-discover external sessions in background so GET response is instant (< 5ms)
	if wsPath != "" {
		go func() {
			_ = m.AutoDiscoverExternalSessions(workspaceID, wsPath)
		}()
	}

	rows, err := m.db.Query(`
		SELECT id, workspace_id, title, status, COALESCE(kanban_status, 'todo'), COALESCE(subtasks_json, '[]'), COALESCE(remote_url, ''), COALESCE(order_idx, 0), COALESCE(result_summary, ''), current_agent, COALESCE(last_handoff_summary, ''), created_at, updated_at
		FROM bridge_sessions WHERE workspace_id = ? ORDER BY order_idx ASC, updated_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}

	var list []BridgeSession
	sessionIndex := make(map[string]int)
	for rows.Next() {
		var s BridgeSession
		var subtasksRaw string
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.Title, &s.Status, &s.KanbanStatus, &subtasksRaw, &s.RemoteURL, &s.OrderIdx, &s.ResultSummary, &s.CurrentAgent, &s.LastHandoffSummary, &s.CreatedAt, &s.UpdatedAt); err == nil {
			s.Workspace = wsPath
			s.Bindings = make(map[string]string)
			s.Tools = []ToolBinding{}
			s.Subtasks = []Subtask{}
			if subtasksRaw != "" {
				_ = json.Unmarshal([]byte(subtasksRaw), &s.Subtasks)
			}
			if s.KanbanStatus == "" {
				s.KanbanStatus = "todo"
			}
			sessionIndex[s.ID] = len(list)
			list = append(list, s)
		}
	}
	_ = rows.Close()

	if len(list) == 0 {
		return list, nil
	}

	// Batch load all bindings in ONE single fast query (resolves N+1 query issue)
	bRows, err := m.db.Query(`
		SELECT b.bridge_session_id, b.agent_name, b.native_session_id, COALESCE(b.last_synced_hash, ''), b.updated_at
		FROM agent_bindings b
		JOIN bridge_sessions s ON s.id = b.bridge_session_id
		WHERE s.workspace_id = ?
		ORDER BY CASE b.agent_name 
			WHEN 'agy' THEN 1 
			WHEN 'claude' THEN 2 
			WHEN 'codex' THEN 3 
			ELSE 4 
		END, b.agent_name ASC
	`, workspaceID)
	if err == nil {
		for bRows.Next() {
			var sessID string
			var tb ToolBinding
			if err := bRows.Scan(&sessID, &tb.AgentName, &tb.NativeSessionID, &tb.LastSyncedHash, &tb.UpdatedAt); err == nil {
				if idx, ok := sessionIndex[sessID]; ok {
					list[idx].Bindings[tb.AgentName] = tb.NativeSessionID

					// Fast metadata lookup without parsing full transcripts
					if tb.NativeSessionID != "" {
						if meta := m.GetNativeMeta(tb.AgentName, tb.NativeSessionID, wsPath); meta != nil {
							tb.Title = meta.Title
							tb.TurnCount = meta.TurnCount
						}
					}
					list[idx].Tools = append(list[idx].Tools, tb)
				}
			}
		}
		_ = bRows.Close()
	}

	// Update task titles if renamed externally or currently default
	for i := range list {
		for _, tb := range list[i].Tools {
			if tb.Title == "" || tb.Title == "New Task" || tb.Title == "Phiên làm việc mới" {
				continue
			}
			cleanTitle := strings.TrimSpace(list[i].Title)
			isDefault := cleanTitle == "" || cleanTitle == "New Task" || cleanTitle == "Phiên làm việc mới"
			isCurrentAgentMismatch := tb.AgentName == list[i].CurrentAgent && tb.Title != list[i].Title

			if isDefault || isCurrentAgentMismatch {
				list[i].Title = tb.Title
				_, _ = m.db.Exec(`UPDATE bridge_sessions SET title = ? WHERE id = ?`, tb.Title, list[i].ID)
				break
			}
		}
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

// UpdateSessionTitle updates the title of a bridge session and syncs the title to bound native agent sessions
func (m *Manager) UpdateSessionTitle(bridgeSessionID, title string) error {
	now := time.Now()
	_, err := m.db.Exec(`UPDATE bridge_sessions SET title = ?, updated_at = ? WHERE id = ?`, title, now, bridgeSessionID)
	if err != nil {
		return err
	}

	var wsPath string
	_ = m.db.QueryRow(`
		SELECT w.path FROM bridge_sessions s 
		JOIN workspaces w ON s.workspace_id = w.id 
		WHERE s.id = ?
	`, bridgeSessionID).Scan(&wsPath)

	rows, err := m.db.Query(`SELECT agent_name, native_session_id FROM agent_bindings WHERE bridge_session_id = ?`, bridgeSessionID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ag, nid string
			if rows.Scan(&ag, &nid) == nil {
				InvalidateNativeMeta(ag, nid)
				if BaseAgent(ag) == "agy" && nid != "" {
					_ = SyncAgyConversationMeta(nid, title, wsPath)
				}
			}
		}
	}
	return nil
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

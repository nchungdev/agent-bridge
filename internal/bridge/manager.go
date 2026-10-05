package bridge

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// BridgeSession is a universal task session
type BridgeSession struct {
	ID                 string    `json:"id"`
	WorkspaceID        string    `json:"workspace_id"`
	Title              string    `json:"title"`
	Status             string    `json:"status"` // active, paused, completed
	CurrentAgent       string    `json:"current_agent"`
	LastHandoffSummary string    `json:"last_handoff_summary"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// AgentBinding maps universal session to native agent session ID
type AgentBinding struct {
	ID              int64     `json:"id"`
	BridgeSessionID string    `json:"bridge_session_id"`
	AgentName       string    `json:"agent_name"`
	NativeSessionID string    `json:"native_session_id"`
	SessionFilePath string    `json:"session_file_path"`
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
	id := fmt.Sprintf("ws_%x", cleanPath)
	if len(id) > 16 {
		id = id[:16]
	}

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

	return &BridgeSession{
		ID:           sessID,
		WorkspaceID:  workspaceID,
		Title:        title,
		Status:       "active",
		CurrentAgent: initialAgent,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// GetWorkspaceSessions returns all sessions for a workspace
func (m *Manager) GetWorkspaceSessions(workspaceID string) ([]BridgeSession, error) {
	rows, err := m.db.Query(`
		SELECT id, workspace_id, title, status, current_agent, COALESCE(last_handoff_summary, ''), created_at, updated_at
		FROM bridge_sessions WHERE workspace_id = ? ORDER BY updated_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []BridgeSession
	for rows.Next() {
		var s BridgeSession
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.Title, &s.Status, &s.CurrentAgent, &s.LastHandoffSummary, &s.CreatedAt, &s.UpdatedAt); err == nil {
			list = append(list, s)
		}
	}
	return list, nil
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

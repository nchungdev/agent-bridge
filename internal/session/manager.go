package session

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Workspace string    `json:"workspace"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MediaItem struct {
	MimeType string `json:"mime_type"`
	URI      string `json:"uri"`
}

type ToolStep struct {
	Name    string `json:"name"`
	Action  string `json:"action"`
	Summary string `json:"summary"`
	Command string `json:"command,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Path    string `json:"path,omitempty"`
	Output  string `json:"output,omitempty"`
	Status  string `json:"status"`
}

type Message struct {
	ID         string      `json:"id"`
	SessionID  string      `json:"session_id"`
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	Agent      string      `json:"agent,omitempty"`
	Model      string      `json:"model,omitempty"`
	TokenCount int         `json:"token_count"`
	Duration   string      `json:"duration,omitempty"`
	Media      []MediaItem `json:"media,omitempty"`
	Steps      []ToolStep  `json:"steps,omitempty"`
	IsRunning  bool        `json:"is_running,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

type Manager struct {
	db *sql.DB
}

func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// CreateSession creates a new conversation session.
func (m *Manager) CreateSession(name, workspace string) (*Session, error) {
	s := &Session{
		ID:        uuid.New().String(),
		Name:      name,
		Workspace: workspace,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_, err := m.db.Exec(
		`INSERT INTO sessions (id, name, workspace, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Workspace, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

// ListSessions returns all sessions ordered by most recently updated.
func (m *Manager) ListSessions() ([]Session, error) {
	rows, err := m.db.Query(`SELECT id, name, workspace, created_at, updated_at FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Name, &s.Workspace, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// RenameSession sets a session's display name.
func (m *Manager) RenameSession(id, name string) error {
	_, err := m.db.Exec(`UPDATE sessions SET name = ?, updated_at = datetime('now') WHERE id = ?`, name, id)
	return err
}

// TouchSession bumps a session to the top of the recency-ordered list.
func (m *Manager) TouchSession(id string) {
	_, _ = m.db.Exec(`UPDATE sessions SET updated_at = datetime('now') WHERE id = ?`, id)
}

// GetSession retrieves a single session by ID.
func (m *Manager) GetSession(id string) (*Session, error) {
	var s Session
	err := m.db.QueryRow(
		`SELECT id, name, workspace, created_at, updated_at FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.Workspace, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &s, nil
}

// DeleteSession removes a session and all its messages.
func (m *Manager) DeleteSession(id string) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec(`DELETE FROM file_diffs WHERE message_id IN (SELECT id FROM messages WHERE session_id = ?)`, id)
	tx.Exec(`DELETE FROM usage_log WHERE message_id IN (SELECT id FROM messages WHERE session_id = ?)`, id)
	tx.Exec(`DELETE FROM messages WHERE session_id = ?`, id)
	tx.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return tx.Commit()
}

// SaveMessage persists a message to the session history.
func (m *Manager) SaveMessage(msg *Message) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	mediaBytes, _ := json.Marshal(msg.Media)
	if mediaBytes == nil {
		mediaBytes = []byte("[]")
	}
	stepsBytes, _ := json.Marshal(msg.Steps)
	if stepsBytes == nil {
		stepsBytes = []byte("[]")
	}

	_, err := m.db.Exec(
		`INSERT INTO messages (id, session_id, role, content, agent, model, token_count, media_json, steps_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, msg.Role, msg.Content, msg.Agent, msg.Model, msg.TokenCount, string(mediaBytes), string(stepsBytes), msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}
	// Touch session updated_at
	m.db.Exec(`UPDATE sessions SET updated_at = datetime('now') WHERE id = ?`, msg.SessionID)
	return nil
}

// GetMessages returns messages for a session, ordered chronologically.
func (m *Manager) GetMessages(sessionID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := m.db.Query(
		`SELECT id, session_id, role, content, COALESCE(agent,''), COALESCE(model,''), token_count, COALESCE(media_json,'[]'), COALESCE(steps_json,'[]'), created_at
		 FROM messages WHERE session_id = ? ORDER BY created_at ASC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var mediaJSON, stepsJSON string
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.Agent, &msg.Model, &msg.TokenCount, &mediaJSON, &stepsJSON, &msg.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(mediaJSON), &msg.Media)
		_ = json.Unmarshal([]byte(stepsJSON), &msg.Steps)
		messages = append(messages, msg)
	}
	return messages, nil
}

// GetContext returns a formatted context string for injecting into CLI prompts.
// It compiles recent messages into a multi-turn conversation format.
func (m *Manager) GetContext(sessionID string, maxMessages int) (string, error) {
	msgs, err := m.GetMessages(sessionID, maxMessages)
	if err != nil {
		return "", err
	}
	if len(msgs) == 0 {
		return "", nil
	}

	var b strings.Builder
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			b.WriteString("Human: ")
		case "assistant":
			b.WriteString("Assistant: ")
		default:
			b.WriteString(msg.Role + ": ")
		}
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}
	return b.String(), nil
}

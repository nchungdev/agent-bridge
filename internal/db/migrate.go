package db

import (
	"database/sql"
	"fmt"
)

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    workspace   TEXT,
    created_at  DATETIME DEFAULT (datetime('now')),
    updated_at  DATETIME DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    role        TEXT NOT NULL,
    content     TEXT NOT NULL,
    agent       TEXT,
    model       TEXT,
    token_count INTEGER DEFAULT 0,
    media_json  TEXT DEFAULT '[]',
    steps_json  TEXT DEFAULT '[]',
    created_at  DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);

CREATE TABLE IF NOT EXISTS file_diffs (
    id          TEXT PRIMARY KEY,
    message_id  TEXT NOT NULL REFERENCES messages(id),
    file_path   TEXT NOT NULL,
    diff_patch  TEXT NOT NULL,
    status      TEXT DEFAULT 'pending',
    created_at  DATETIME DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS agent_configs (
    id          TEXT PRIMARY KEY,
    agent       TEXT NOT NULL,
    auth_type   TEXT NOT NULL,
    config_data TEXT NOT NULL,
    is_active   INTEGER DEFAULT 1,
    priority    INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS usage_log (
    id          TEXT PRIMARY KEY,
    message_id  TEXT REFERENCES messages(id),
    agent       TEXT NOT NULL,
    model       TEXT NOT NULL,
    input_tokens  INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cost_usd    REAL DEFAULT 0,
    created_at  DATETIME DEFAULT (datetime('now'))
);

-- Agent Bridge: Workspaces
CREATE TABLE IF NOT EXISTS workspaces (
    id TEXT PRIMARY KEY,
    path TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    active_bridge_session_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Agent Bridge: Universal Bridge Sessions
CREATE TABLE IF NOT EXISTS bridge_sessions (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT DEFAULT 'active',
    current_agent TEXT NOT NULL,
    last_handoff_summary TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

-- Agent Bridge: Mappings between Universal Session and Native Agent IDs
CREATE TABLE IF NOT EXISTS agent_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bridge_session_id TEXT NOT NULL,
    agent_name TEXT NOT NULL,
    native_session_id TEXT NOT NULL,
    session_file_path TEXT,
    is_current INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(bridge_session_id, agent_name),
    FOREIGN KEY (bridge_session_id) REFERENCES bridge_sessions(id) ON DELETE CASCADE
);

-- Agent Bridge: Handoff Checkpoints
CREATE TABLE IF NOT EXISTS handoff_checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bridge_session_id TEXT NOT NULL,
    from_agent TEXT NOT NULL,
    to_agent TEXT NOT NULL,
    from_native_id TEXT,
    to_native_id TEXT,
    trigger_reason TEXT,
    task_goal TEXT,
    git_diff_summary TEXT,
    context_snapshot TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (bridge_session_id) REFERENCES bridge_sessions(id) ON DELETE CASCADE
);

-- Agent Bridge: CLI Engines Status
CREATE TABLE IF NOT EXISTS cli_engines (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    binary_path TEXT,
    is_installed INTEGER DEFAULT 0,
    install_command TEXT,
    auth_status TEXT,
    active_account TEXT,
    last_health_check DATETIME
);
`

func Migrate(db *sql.DB) error {
	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	// Safe backward-compatible column migrations
	_, _ = db.Exec(`ALTER TABLE messages ADD COLUMN media_json TEXT DEFAULT '[]'`)
	_, _ = db.Exec(`ALTER TABLE messages ADD COLUMN steps_json TEXT DEFAULT '[]'`)
	return nil
}

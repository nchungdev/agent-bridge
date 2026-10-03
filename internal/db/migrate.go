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

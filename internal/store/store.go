// Package store is the shared ContextStore: an append-only event log plus
// per-conversation bindings, approvals and working state, kept in SQLite next
// to the existing sessions/messages tables (which are left untouched).
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/nchungdev/agent-hub/internal/core"
)

const schema = `
CREATE TABLE IF NOT EXISTS v2_conv (
    conv_id       TEXT PRIMARY KEY,
    active_engine TEXT,
    mode          TEXT,
    workspace     TEXT,
    updated_at    DATETIME DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS v2_events (
    conv_id TEXT NOT NULL,
    seq     INTEGER NOT NULL,
    engine  TEXT,
    model   TEXT,
    type    TEXT NOT NULL,
    payload TEXT NOT NULL,
    ts      DATETIME DEFAULT (datetime('now')),
    PRIMARY KEY (conv_id, seq)
);
CREATE TABLE IF NOT EXISTS v2_bindings (
    conv_id           TEXT NOT NULL,
    engine            TEXT NOT NULL,
    engine_session_id TEXT,
    state             TEXT NOT NULL,
    last_synced_seq   INTEGER DEFAULT 0,
    updated_at        DATETIME DEFAULT (datetime('now')),
    PRIMARY KEY (conv_id, engine)
);
CREATE TABLE IF NOT EXISTS v2_approvals (
    id         TEXT PRIMARY KEY,
    conv_id    TEXT NOT NULL,
    engine     TEXT,
    tool       TEXT,
    args       TEXT,
    risk       TEXT,
    decision   TEXT,
    scope      TEXT,
    decided_by TEXT,
    created_at DATETIME DEFAULT (datetime('now')),
    decided_at DATETIME
);
CREATE TABLE IF NOT EXISTS v2_conv_meta (
    conv_id  TEXT PRIMARY KEY,
    pinned   INTEGER DEFAULT 0,
    archived INTEGER DEFAULT 0,
    grp      TEXT DEFAULT '',
    unread   INTEGER DEFAULT 0
);
CREATE TABLE IF NOT EXISTS v2_working_state (
    conv_id TEXT PRIMARY KEY,
    json    TEXT NOT NULL,
    rev     INTEGER DEFAULT 0,
    ts      DATETIME DEFAULT (datetime('now'))
);
`

type Store struct{ db *sql.DB }

func New(db *sql.DB) (*Store, error) {
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate v2 schema: %w", err)
	}
	// additive column migrations (errors mean the column already exists)
	_, _ = db.Exec(`ALTER TABLE v2_conv ADD COLUMN model TEXT`)
	_, _ = db.Exec(`ALTER TABLE v2_conv ADD COLUMN effort TEXT`)
	_, _ = db.Exec(`ALTER TABLE v2_conv_meta ADD COLUMN title TEXT DEFAULT ''`)
	return &Store{db: db}, nil
}

var secretRes = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{16,}`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password)["']?\s*[:=]\s*["']?)[^\s"',}]{6,}`),
}

// Redact masks obvious credentials before anything is persisted.
func Redact(s string) string {
	for i, re := range secretRes {
		if i == len(secretRes)-1 {
			s = re.ReplaceAllString(s, "${1}[REDACTED]")
		} else {
			s = re.ReplaceAllString(s, "[REDACTED]")
		}
	}
	return s
}

// Append stores an event, assigning the next per-conversation sequence number.
func (s *Store) Append(ev core.Event) (core.Event, error) {
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return ev, err
	}
	payload := Redact(string(body))
	tx, err := s.db.Begin()
	if err != nil {
		return ev, err
	}
	defer tx.Rollback()
	var seq int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(seq),0)+1 FROM v2_events WHERE conv_id=?`, ev.ConvID).Scan(&seq); err != nil {
		return ev, err
	}
	ev.Seq = seq
	if _, err := tx.Exec(`INSERT INTO v2_events(conv_id,seq,engine,model,type,payload,ts) VALUES(?,?,?,?,?,?,?)`,
		ev.ConvID, seq, ev.Engine, ev.Model, string(ev.Type), payload, ev.Time.Format(time.RFC3339Nano)); err != nil {
		return ev, err
	}
	return ev, tx.Commit()
}

func scanEvents(rows *sql.Rows) ([]core.Event, error) {
	defer rows.Close()
	var out []core.Event
	for rows.Next() {
		var seq int64
		var payload string
		if err := rows.Scan(&seq, &payload); err != nil {
			return nil, err
		}
		var ev core.Event
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			return nil, err
		}
		ev.Seq = seq
		out = append(out, ev)
	}
	return out, rows.Err()
}

// Since returns events with seq > after, oldest first.
func (s *Store) Since(conv string, after int64, limit int) ([]core.Event, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(`SELECT seq,payload FROM v2_events WHERE conv_id=? AND seq>? ORDER BY seq LIMIT ?`, conv, after, limit)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// Tail returns the last n events, oldest first.
func (s *Store) Tail(conv string, n int) ([]core.Event, error) {
	rows, err := s.db.Query(`SELECT seq,payload FROM (SELECT seq,payload FROM v2_events WHERE conv_id=? ORDER BY seq DESC LIMIT ?) ORDER BY seq`, conv, n)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// Search does a simple substring match over payloads.
func (s *Store) Search(conv, q string, limit int) ([]core.Event, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT seq,payload FROM v2_events WHERE conv_id=? AND payload LIKE ? ORDER BY seq DESC LIMIT ?`, conv, "%"+q+"%", limit)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

type Binding struct {
	ConvID          string
	Engine          string
	EngineSessionID string
	State           core.State
	LastSyncedSeq   int64
}

func (s *Store) UpsertBinding(b Binding) error {
	_, err := s.db.Exec(`INSERT INTO v2_bindings(conv_id,engine,engine_session_id,state,last_synced_seq,updated_at)
		VALUES(?,?,?,?,?,datetime('now'))
		ON CONFLICT(conv_id,engine) DO UPDATE SET engine_session_id=excluded.engine_session_id,
		state=excluded.state,last_synced_seq=excluded.last_synced_seq,updated_at=datetime('now')`,
		b.ConvID, b.Engine, b.EngineSessionID, string(b.State), b.LastSyncedSeq)
	return err
}

func (s *Store) GetBinding(conv, engine string) (*Binding, error) {
	var b Binding
	var st string
	err := s.db.QueryRow(`SELECT conv_id,engine,COALESCE(engine_session_id,''),state,COALESCE(last_synced_seq,0)
		FROM v2_bindings WHERE conv_id=? AND engine=?`, conv, engine).Scan(&b.ConvID, &b.Engine, &b.EngineSessionID, &st, &b.LastSyncedSeq)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.State = core.State(st)
	return &b, nil
}

func (s *Store) ListBindings(conv string) ([]Binding, error) {
	rows, err := s.db.Query(`SELECT conv_id,engine,COALESCE(engine_session_id,''),state,COALESCE(last_synced_seq,0)
		FROM v2_bindings WHERE conv_id=? ORDER BY engine`, conv)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Binding
	for rows.Next() {
		var b Binding
		var st string
		if err := rows.Scan(&b.ConvID, &b.Engine, &b.EngineSessionID, &st, &b.LastSyncedSeq); err != nil {
			return nil, err
		}
		b.State = core.State(st)
		out = append(out, b)
	}
	return out, rows.Err()
}

// ResetLiveBindings marks every binding that claims a live process as suspended.
// Called once at startup: after a hub restart no child process survives.
func (s *Store) ResetLiveBindings() (int64, error) {
	res, err := s.db.Exec(`UPDATE v2_bindings SET state='suspended',updated_at=datetime('now')
		WHERE state IN ('starting','idle','running','awaiting_approval','resuming')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type ConvSettings struct {
	ConvID       string
	ActiveEngine string
	Mode         string
	Workspace    string
	Model        string
	Effort       string
}

func (s *Store) SetConv(c ConvSettings) error {
	_, err := s.db.Exec(`INSERT INTO v2_conv(conv_id,active_engine,mode,workspace,model,effort,updated_at) VALUES(?,?,?,?,?,?,datetime('now'))
		ON CONFLICT(conv_id) DO UPDATE SET active_engine=excluded.active_engine,mode=excluded.mode,
		workspace=excluded.workspace,model=excluded.model,effort=excluded.effort,updated_at=datetime('now')`,
		c.ConvID, c.ActiveEngine, c.Mode, c.Workspace, c.Model, c.Effort)
	return err
}

func (s *Store) GetConv(conv string) (*ConvSettings, error) {
	var c ConvSettings
	err := s.db.QueryRow(`SELECT conv_id,COALESCE(active_engine,''),COALESCE(mode,''),COALESCE(workspace,''),COALESCE(model,''),COALESCE(effort,'') FROM v2_conv WHERE conv_id=?`, conv).
		Scan(&c.ConvID, &c.ActiveEngine, &c.Mode, &c.Workspace, &c.Model, &c.Effort)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &c, err
}

// Approvals

func (s *Store) AddApproval(conv, engine string, r core.ApprovalRequest) error {
	args, _ := json.Marshal(r.Args)
	_, err := s.db.Exec(`INSERT OR REPLACE INTO v2_approvals(id,conv_id,engine,tool,args,risk) VALUES(?,?,?,?,?,?)`,
		r.ID, conv, engine, r.Tool, Redact(string(args)), r.Risk)
	return err
}

func (s *Store) ResolveApproval(id string, d core.Decision) error {
	dec := "deny"
	if d.Allow {
		dec = "allow"
	}
	res, err := s.db.Exec(`UPDATE v2_approvals SET decision=?,scope=?,decided_by=?,decided_at=datetime('now') WHERE id=? AND decision IS NULL`,
		dec, d.Scope, d.DecidedBy, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return core.ErrNoApproval
	}
	return nil
}

// PendingApprovals lists unresolved approvals for a conversation (survives GUI reconnects).
func (s *Store) PendingApprovals(conv string) ([]core.ApprovalRequest, error) {
	rows, err := s.db.Query(`SELECT id,COALESCE(tool,''),COALESCE(args,'{}'),COALESCE(risk,'') FROM v2_approvals WHERE conv_id=? AND decision IS NULL ORDER BY created_at`, conv)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.ApprovalRequest
	for rows.Next() {
		var r core.ApprovalRequest
		var args string
		if err := rows.Scan(&r.ID, &r.Tool, &args, &r.Risk); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(args), &r.Args)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Working state

type WorkingState struct {
	ActiveEngine  string   `json:"active_engine,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	FilesChanged  []string `json:"files_changed,omitempty"`
	LastError     string   `json:"last_error,omitempty"`
	ApprovedTools []string `json:"approved_tools,omitempty"`
	Goal          string   `json:"goal,omitempty"`
	Notes         []string `json:"notes,omitempty"`
}

func (s *Store) GetState(conv string) (WorkingState, error) {
	var ws WorkingState
	var js string
	err := s.db.QueryRow(`SELECT json FROM v2_working_state WHERE conv_id=?`, conv).Scan(&js)
	if err == sql.ErrNoRows {
		return ws, nil
	}
	if err != nil {
		return ws, err
	}
	return ws, json.Unmarshal([]byte(js), &ws)
}

func (s *Store) SetState(conv string, ws WorkingState) error {
	b, err := json.Marshal(ws)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO v2_working_state(conv_id,json,rev,ts) VALUES(?,?,1,datetime('now'))
		ON CONFLICT(conv_id) DO UPDATE SET json=excluded.json,rev=rev+1,ts=datetime('now')`, conv, Redact(string(b)))
	return err
}

func addUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// ApplyEvent updates working state by rule (no LLM): files touched, last error, approvals.
func (s *Store) ApplyEvent(ev core.Event) error {
	ws, err := s.GetState(ev.ConvID)
	if err != nil {
		return err
	}
	changed := false
	switch ev.Type {
	case core.EvDiff:
		if ev.Diff != nil && ev.Diff.File != "" {
			ws.FilesChanged = addUnique(ws.FilesChanged, ev.Diff.File)
			changed = true
		}
	case core.EvError:
		if ev.Err != nil {
			ws.LastError = ev.Err.Message
			changed = true
		}
	case core.EvEngineSwitch:
		if to, ok := ev.Data["to"].(string); ok {
			ws.ActiveEngine = to
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.SetState(ev.ConvID, ws)
}

// ApprovalTool returns the tool name recorded for an approval id.
func (s *Store) ApprovalTool(id string) (string, error) {
	var t sql.NullString
	err := s.db.QueryRow(`SELECT tool FROM v2_approvals WHERE id=?`, id).Scan(&t)
	return t.String, err
}

// Conversation list metadata (pin / archive / group / unread) ---------------

type Meta struct {
	Pinned   bool   `json:"pinned"`
	Archived bool   `json:"archived"`
	Group    string `json:"group"`
	Unread   bool   `json:"unread"`
	// Title overrides the display name of read-only items (e.g. Antigravity history).
	Title string `json:"title"`
}

// MetaPatch updates only the fields that are set.
type MetaPatch struct {
	Pinned   *bool
	Archived *bool
	Group    *string
	Unread   *bool
	Title    *string
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) UpdateMeta(conv string, p MetaPatch) error {
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO v2_conv_meta(conv_id) VALUES(?)`, conv); err != nil {
		return err
	}
	if p.Pinned != nil {
		if _, err := s.db.Exec(`UPDATE v2_conv_meta SET pinned=? WHERE conv_id=?`, b2i(*p.Pinned), conv); err != nil {
			return err
		}
	}
	if p.Archived != nil {
		if _, err := s.db.Exec(`UPDATE v2_conv_meta SET archived=? WHERE conv_id=?`, b2i(*p.Archived), conv); err != nil {
			return err
		}
	}
	if p.Group != nil {
		if _, err := s.db.Exec(`UPDATE v2_conv_meta SET grp=? WHERE conv_id=?`, *p.Group, conv); err != nil {
			return err
		}
	}
	if p.Unread != nil {
		if _, err := s.db.Exec(`UPDATE v2_conv_meta SET unread=? WHERE conv_id=?`, b2i(*p.Unread), conv); err != nil {
			return err
		}
	}
	if p.Title != nil {
		if _, err := s.db.Exec(`UPDATE v2_conv_meta SET title=? WHERE conv_id=?`, *p.Title, conv); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListMeta() (map[string]Meta, error) {
	rows, err := s.db.Query(`SELECT conv_id,pinned,archived,COALESCE(grp,''),unread,COALESCE(title,'') FROM v2_conv_meta`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Meta{}
	for rows.Next() {
		var id string
		var p, a, u int
		var m Meta
		if err := rows.Scan(&id, &p, &a, &m.Group, &u, &m.Title); err != nil {
			return nil, err
		}
		m.Pinned, m.Archived, m.Unread = p == 1, a == 1, u == 1
		out[id] = m
	}
	return out, rows.Err()
}

// DeleteConv removes every v2 record of a conversation.
func (s *Store) DeleteConv(conv string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM v2_events WHERE conv_id=?`, `DELETE FROM v2_bindings WHERE conv_id=?`,
		`DELETE FROM v2_approvals WHERE conv_id=?`, `DELETE FROM v2_working_state WHERE conv_id=?`,
		`DELETE FROM v2_conv WHERE conv_id=?`, `DELETE FROM v2_conv_meta WHERE conv_id=?`,
	} {
		if _, err := tx.Exec(q, conv); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ForkEvents copies a conversation's transcript into dst (fresh sequence numbers).
// Approval and state events are skipped: a fork must not inherit pending prompts.
func (s *Store) ForkEvents(src, dst string) (int, error) {
	evs, err := s.Since(src, 0, 1_000_000)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range evs {
		switch e.Type {
		case core.EvApprovalRequest, core.EvApprovalResolved, core.EvStateChange:
			continue
		}
		e.ConvID, e.Seq = dst, 0
		if _, err := s.Append(e); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

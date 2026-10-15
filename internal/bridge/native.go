package bridge

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nchungdev/agent-bridge/internal/session"
)

// NativeTurn is one user/assistant message from a CLI's own transcript.
type NativeTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// NativeSession is a conversation stored by a CLI itself (not by the bridge).
type NativeSession struct {
	Agent         string       `json:"agent"`
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Workspace     string       `json:"workspace"`
	UpdatedAt     time.Time    `json:"updated_at"`
	LastUser      string       `json:"last_user"`
	LastAssistant string       `json:"last_assistant"`
	TurnCount     int          `json:"turn_count"`
	Turns         []NativeTurn `json:"turns,omitempty"`
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// claudeProjectDir mirrors Claude Code's folder naming: every non-alphanumeric char becomes "-".
func claudeProjectDir(workspace string) string {
	return filepath.Join(homeDir(), ".claude", "projects", nonAlnum.ReplaceAllString(filepath.Clean(workspace), "-"))
}

// ---------- Antigravity ----------

func agyWorkspaceMatches(uris, workspace string) bool {
	ws := filepath.Clean(workspace)
	for _, p := range agyWorkspacesFromURIs(uris) {
		if p == ws {
			return true
		}
	}
	return false
}

func agyWorkspacesFromURIs(uris string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(uris, func(r rune) bool { return r == ',' || r == '"' || r == '[' || r == ']' }) {
		p := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "file://"))
		if dec, err := url.PathUnescape(p); err == nil {
			p = dec
		}
		if strings.HasPrefix(p, "/") {
			out = append(out, filepath.Clean(p))
		}
	}
	return out
}

func openAgySummaries() (*sql.DB, error) {
	dbPath := filepath.Join(homeDir(), ".gemini/antigravity-cli/conversation_summaries.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}
	return sql.Open("sqlite", dbPath+"?mode=ro")
}

// FindAgyProjectID looks up the Antigravity project ID for a given workspace path from conversation_summaries.db.
func FindAgyProjectID(workspace string) string {
	if workspace == "" {
		return ""
	}
	db, err := openAgySummaries()
	if err != nil {
		return filepath.Base(workspace)
	}
	defer db.Close()

	cleanWS := filepath.Clean(workspace)
	encodedWS := strings.ReplaceAll(cleanWS, " ", "%20")
	var projectID string
	err = db.QueryRow(`
		SELECT project_id FROM conversation_summaries 
		WHERE (workspace_uris LIKE ? OR workspace_uris LIKE ?) 
		  AND project_id != '' 
		  AND project_id != 'default-cli-project'
		ORDER BY last_modified_time DESC LIMIT 1
	`, "%"+cleanWS+"%", "%"+encodedWS+"%").Scan(&projectID)
	if err == nil && projectID != "" {
		return projectID
	}
	return filepath.Base(workspace)
}

// SyncAgyConversationMeta updates title, project_id, and workspace_uris for a conversation in Antigravity's DB.
func SyncAgyConversationMeta(convID, title, workspace string) error {
	if convID == "" {
		return nil
	}
	dbPath := filepath.Join(homeDir(), ".gemini/antigravity-cli/conversation_summaries.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=5000")
	if err != nil {
		return err
	}
	defer db.Close()

	projID := FindAgyProjectID(workspace)
	cleanWS := filepath.Clean(workspace)
	wsURI := fmt.Sprintf(`["file://%s"]`, strings.ReplaceAll(cleanWS, " ", "%20"))

	var sets []string
	var args []any

	if title != "" && title != "New Task" && title != "Phiên làm việc mới" {
		sets = append(sets, "title = ?")
		args = append(args, title)
	}
	if projID != "" {
		sets = append(sets, "project_id = ?")
		args = append(args, projID)
	}
	if workspace != "" {
		sets = append(sets, "workspace_uris = ?")
		args = append(args, wsURI)
	}
	if len(sets) == 0 {
		return nil
	}

	query := "UPDATE conversation_summaries SET " + strings.Join(sets, ", ") + " WHERE conversation_id = ?"
	args = append(args, convID)
	_, err = db.Exec(query, args...)
	return err
}

func listAgySessions(workspace string, limit int) []NativeSession {
	db, err := openAgySummaries()
	if err != nil {
		return nil
	}
	defer db.Close()

	projID := FindAgyProjectID(workspace)
	rows, err := db.Query(`SELECT conversation_id, title, COALESCE(workspace_uris,''), COALESCE(project_id,''), COALESCE(step_count, 0), COALESCE(last_modified_time,'')
		FROM conversation_summaries ORDER BY last_modified_time DESC LIMIT 300`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []NativeSession
	for rows.Next() && len(out) < limit {
		var id, title, uris, pid, mod string
		var stepCount int
		if rows.Scan(&id, &title, &uris, &pid, &stepCount, &mod) != nil {
			continue
		}
		matches := agyWorkspaceMatches(uris, workspace)
		if !matches && projID != "" && projID != "default-cli-project" && pid == projID {
			matches = true
		}
		if !matches {
			continue
		}
		title = strings.TrimSpace(title)
		if title == "" && stepCount == 0 {
			continue
		}
		s := NativeSession{
			Agent:     "agy",
			ID:        id,
			Title:     title,
			Workspace: workspace,
			UpdatedAt: parseAnyTime(mod),
			TurnCount: stepCount,
		}
		if s.Title == "" || s.TurnCount == 0 {
			if turns := readAgyTurns(id); len(turns) > 0 {
				fillSummary(&s, turns)
				if s.Title == "" {
					s.Title = clip(firstUser(turns), 70)
				}
			}
		}
		if s.Title == "" {
			s.Title = "Phiên làm việc mới"
		}
		out = append(out, s)
	}
	return out
}

func readAgyTurns(id string) []NativeTurn {
	msgs, err := session.ReadAntigravityTranscript(id)
	if err != nil {
		return nil
	}
	var turns []NativeTurn
	for _, m := range msgs {
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		turns = append(turns, NativeTurn{Role: m.Role, Content: m.Content})
	}
	return turns
}

// ---------- Claude Code ----------

// readClaudeSessionMeta reads only the first few lines of a Claude JSONL file to get the first user message.
func readClaudeSessionMeta(path string) (title, cwd string, turns int) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 256*1024), 1024*1024)
	for i := 0; sc.Scan() && i < 100; i++ {
		var line struct {
			Type    string          `json:"type"`
			Cwd     string          `json:"cwd"`
			Message json.RawMessage `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if cwd == "" && line.Cwd != "" {
			cwd = line.Cwd
		}
		if title == "" && line.Type == "user" && len(line.Message) > 0 {
			var msg struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			}
			if json.Unmarshal(line.Message, &msg) == nil {
				text := claudeText(msg.Content)
				if text != "" && !strings.HasPrefix(text, "<command-") && !strings.HasPrefix(text, "<local-command") {
					title = clip(text, 70)
				}
			}
		}
		if title != "" && cwd != "" {
			break
		}
	}
	return title, cwd, 1
}

func listClaudeSessions(workspace string, limit int) []NativeSession {
	dir := claudeProjectDir(workspace)
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	sort.Slice(files, func(i, j int) bool { return modTime(files[i]).After(modTime(files[j])) })
	var out []NativeSession
	for _, f := range files {
		if len(out) >= limit {
			break
		}
		title, cwd, _ := readClaudeSessionMeta(f)
		if title == "" {
			continue
		}
		if cwd != "" && filepath.Clean(cwd) != filepath.Clean(workspace) {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		out = append(out, NativeSession{
			Agent:     "claude",
			ID:        id,
			Title:     title,
			Workspace: workspace,
			UpdatedAt: modTime(f),
			TurnCount: 1,
		})
	}
	return out
}

func readClaudeTurns(path string) ([]NativeTurn, string) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 20*1024*1024)
	var turns []NativeTurn
	cwd := ""
	for sc.Scan() {
		var line struct {
			Type        string          `json:"type"`
			IsSidechain bool            `json:"isSidechain"`
			Cwd         string          `json:"cwd"`
			IsMeta      bool            `json:"isMeta"`
			Message     json.RawMessage `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if cwd == "" && line.Cwd != "" {
			cwd = line.Cwd
		}
		if line.IsSidechain || line.IsMeta || (line.Type != "user" && line.Type != "assistant") || len(line.Message) == 0 {
			continue
		}
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(line.Message, &msg) != nil {
			continue
		}
		text := claudeText(msg.Content)
		if text == "" || strings.HasPrefix(text, "<command-") || strings.HasPrefix(text, "<local-command") {
			continue
		}
		turns = append(turns, NativeTurn{Role: msg.Role, Content: text})
	}
	return turns, cwd
}

func claudeText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// readCodexSessionMeta reads only the first few lines of a Codex JSONL file to get the first user message.
func readCodexSessionMeta(path string) (title, cwd, id string) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 256*1024), 1024*1024)
	for i := 0; sc.Scan() && i < 100; i++ {
		var line struct {
			Type    string `json:"type"`
			Payload struct {
				ID      string `json:"id"`
				Cwd     string `json:"cwd"`
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if line.Type == "session_meta" {
			cwd, id = line.Payload.Cwd, line.Payload.ID
		}
		if title == "" && line.Payload.Type == "message" && line.Payload.Role == "user" {
			var parts []string
			for _, c := range line.Payload.Content {
				if strings.TrimSpace(c.Text) != "" {
					parts = append(parts, c.Text)
				}
			}
			text := strings.TrimSpace(strings.Join(parts, "\n"))
			if text != "" && !strings.HasPrefix(text, "<environment_context>") && !strings.HasPrefix(text, "<user_instructions>") {
				title = clip(text, 70)
			}
		}
		if title != "" && cwd != "" && id != "" {
			break
		}
	}
	return title, cwd, id
}

func listCodexSessions(workspace string, limit int) []NativeSession {
	root := filepath.Join(homeDir(), ".codex", "sessions")
	var files []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".jsonl") {
			files = append(files, p)
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return modTime(files[i]).After(modTime(files[j])) })
	var out []NativeSession
	for i, f := range files {
		if len(out) >= limit || i > 200 {
			break
		}
		title, cwd, id := readCodexSessionMeta(f)
		if title == "" || filepath.Clean(cwd) != filepath.Clean(workspace) {
			continue
		}
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(f), ".jsonl")
		}
		s := NativeSession{Agent: "codex", ID: id, Title: title, Workspace: workspace, UpdatedAt: modTime(f), TurnCount: 1}
		out = append(out, s)
	}
	return out
}

func readCodexTurns(path string) ([]NativeTurn, string, string) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 20*1024*1024)
	var turns []NativeTurn
	cwd, id := "", ""
	for sc.Scan() {
		var line struct {
			Type    string `json:"type"`
			Payload struct {
				ID      string `json:"id"`
				Cwd     string `json:"cwd"`
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if line.Type == "session_meta" {
			cwd, id = line.Payload.Cwd, line.Payload.ID
			continue
		}
		if line.Payload.Type != "message" || (line.Payload.Role != "user" && line.Payload.Role != "assistant") {
			continue
		}
		var parts []string
		for _, c := range line.Payload.Content {
			if strings.TrimSpace(c.Text) != "" {
				parts = append(parts, c.Text)
			}
		}
		text := strings.TrimSpace(strings.Join(parts, "\n"))
		if text == "" || strings.HasPrefix(text, "<environment_context>") || strings.HasPrefix(text, "<user_instructions>") {
			continue
		}
		turns = append(turns, NativeTurn{Role: line.Payload.Role, Content: text})
	}
	return turns, cwd, id
}

// ---------- shared ----------

func fillSummary(s *NativeSession, turns []NativeTurn) {
	s.TurnCount = len(turns)
	for i := len(turns) - 1; i >= 0; i-- {
		if s.LastUser == "" && turns[i].Role == "user" {
			s.LastUser = clip(turns[i].Content, 220)
		}
		if s.LastAssistant == "" && turns[i].Role == "assistant" {
			s.LastAssistant = clip(turns[i].Content, 300)
		}
		if s.LastUser != "" && s.LastAssistant != "" {
			break
		}
	}
}

func firstUser(turns []NativeTurn) string {
	for _, t := range turns {
		if t.Role == "user" {
			return t.Content
		}
	}
	return "Untitled"
}

func modTime(p string) time.Time {
	if st, err := os.Stat(p); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}

func parseAnyTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05.999999999-07:00", time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ListNativeSessions merges the sessions every CLI stored for this folder, newest first.
func (m *Manager) ListNativeSessions(workspace string) []NativeSession {
	var all []NativeSession
	all = append(all, listAgySessions(workspace, 15)...)
	all = append(all, listClaudeSessions(workspace, 15)...)
	all = append(all, listCodexSessions(workspace, 15)...)
	sort.Slice(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
	return all
}

// NativeMeta holds lightweight metadata for display in task bindings
type NativeMeta struct {
	Title     string
	TurnCount int
	FetchedAt time.Time
}

var nativeMetaCache sync.Map // string -> *NativeMeta

// InvalidateNativeMeta removes cached metadata for a session so subsequent lookups re-fetch immediately.
func InvalidateNativeMeta(agent, id string) {
	nativeMetaCache.Delete(agent + ":" + id)
}

// GetNativeMeta quickly resolves title and turn count without loading full transcripts.
func (m *Manager) GetNativeMeta(agent, id, workspace string) *NativeMeta {
	if id == "" {
		return nil
	}
	key := agent + ":" + id
	if val, ok := nativeMetaCache.Load(key); ok {
		cached := val.(*NativeMeta)
		if time.Since(cached.FetchedAt) < 10*time.Second {
			return cached
		}
	}

	meta := &NativeMeta{FetchedAt: time.Now()}
	switch agent {
	case "agy":
		if db, err := openAgySummaries(); err == nil {
			var t string
			var steps int
			_ = db.QueryRow(`SELECT title, COALESCE(step_count, 0) FROM conversation_summaries WHERE conversation_id = ?`, id).Scan(&t, &steps)
			_ = db.Close()
			meta.Title = strings.TrimSpace(t)
			meta.TurnCount = steps
		}
	case "claude":
		title, _, count := readClaudeSessionMeta(filepath.Join(claudeProjectDir(workspace), id+".jsonl"))
		meta.Title = title
		meta.TurnCount = count
	case "codex":
		root := filepath.Join(homeDir(), ".codex", "sessions")
		var targetFile string
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.Contains(p, id) && strings.HasSuffix(p, ".jsonl") {
				targetFile = p
				return filepath.SkipAll
			}
			return nil
		})
		if targetFile != "" {
			title, _, _ := readCodexSessionMeta(targetFile)
			meta.Title = title
			meta.TurnCount = 1
		}
	}

	if meta.Title != "" || meta.TurnCount > 0 {
		nativeMetaCache.Store(key, meta)
	}
	return meta
}

// GetNativeSession returns one session with its full turn list.
func (m *Manager) GetNativeSession(agent, id, workspace string) *NativeSession {
	var turns []NativeTurn
	var nativeTitle string
	switch agent {
	case "agy":
		turns = readAgyTurns(id)
		if db, err := openAgySummaries(); err == nil {
			_ = db.QueryRow(`SELECT title FROM conversation_summaries WHERE conversation_id = ?`, id).Scan(&nativeTitle)
			_ = db.Close()
		}
	case "claude":
		turns, _ = readClaudeTurns(filepath.Join(claudeProjectDir(workspace), id+".jsonl"))
	case "codex":
		for _, s := range listCodexSessions(workspace, 50) {
			if s.ID == id {
				// re-read full transcript
				_ = filepath.Walk(filepath.Join(homeDir(), ".codex", "sessions"), func(p string, info os.FileInfo, err error) error {
					if err == nil && !info.IsDir() && strings.Contains(p, id) {
						turns, _, _ = readCodexTurns(p)
					}
					return nil
				})
				break
			}
		}
	}
	if len(turns) == 0 && nativeTitle == "" {
		return nil
	}
	s := &NativeSession{Agent: agent, ID: id, Workspace: workspace, Turns: turns}
	fillSummary(s, turns)
	if strings.TrimSpace(nativeTitle) != "" {
		s.Title = strings.TrimSpace(nativeTitle)
	} else {
		s.Title = clip(firstUser(turns), 70)
	}
	return s
}

// DiscoverWorkspaces finds folders any CLI has worked in, so the user never has to register them.
func (m *Manager) DiscoverWorkspaces() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "" || p == "/" || seen[p] {
			return
		}
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	if db, err := openAgySummaries(); err == nil {
		if rows, err := db.Query(`SELECT COALESCE(workspace_uris,'') FROM conversation_summaries ORDER BY last_modified_time DESC LIMIT 500`); err == nil {
			for rows.Next() {
				var uris string
				if rows.Scan(&uris) == nil {
					for _, p := range agyWorkspacesFromURIs(uris) {
						add(p)
					}
				}
			}
			rows.Close()
		}
		db.Close()
	}
	dirs, _ := filepath.Glob(filepath.Join(homeDir(), ".claude", "projects", "*"))
	for _, d := range dirs {
		files, _ := filepath.Glob(filepath.Join(d, "*.jsonl"))
		for _, f := range files {
			if _, cwd := readClaudeTurnsHead(f); cwd != "" {
				add(cwd)
				break
			}
		}
	}
	if def := DefaultWorkspacePath(); def != "" {
		add(def)
	}
	return out
}

// readClaudeTurnsHead reads only until the first cwd is found.
func readClaudeTurnsHead(path string) (struct{}, string) {
	f, err := os.Open(path)
	if err != nil {
		return struct{}{}, ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 20*1024*1024)
	for i := 0; sc.Scan() && i < 50; i++ {
		var line struct {
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(sc.Bytes(), &line) == nil && line.Cwd != "" {
			return struct{}{}, line.Cwd
		}
	}
	return struct{}{}, ""
}

// FormatTurns renders the tail of a transcript for the handoff file.
func FormatTurns(turns []NativeTurn, max int) string {
	if len(turns) > max {
		turns = turns[len(turns)-max:]
	}
	var b strings.Builder
	for _, t := range turns {
		who := "User"
		if t.Role == "assistant" {
			who = "Assistant"
		}
		b.WriteString("**" + who + "**: " + clip(t.Content, 1500) + "\n\n")
	}
	return b.String()
}

// HandoffPrompt is the first message a target CLI gets when it starts from a handoff.
const HandoffPrompt = "Đọc .agent/handoff.md và tiếp tục công việc dang dở."

var (
	nativeIDRe  = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	shellSafeRe = regexp.MustCompile(`^[A-Za-z0-9._:/=-]+$`)
)

// BaseAgent returns the primary agent type ("claude", "agy", "codex") for a given agent ID or variant.
func BaseAgent(agent string) string {
	a := strings.ToLower(strings.TrimSpace(agent))
	if idx := strings.Index(a, "@"); idx >= 0 {
		a = a[:idx]
	}
	if a == "claude" || strings.HasPrefix(a, "claude-") || strings.HasPrefix(a, "claude_") {
		return "claude"
	}
	if a == "agy" || strings.HasPrefix(a, "agy-") || strings.HasPrefix(a, "agy_") ||
		a == "antigravity" || strings.HasPrefix(a, "antigravity-") || strings.HasPrefix(a, "antigravity_") {
		return "agy"
	}
	if a == "codex" || strings.HasPrefix(a, "codex-") || strings.HasPrefix(a, "codex_") {
		return "codex"
	}
	return a
}

// LaunchArgv is the argv that continues work in the target CLI: it resumes the CLI's own
// session when sameAgentID is set, otherwise starts a new session seeded with the handoff prompt.
// Returns nil for an unknown agent or a malformed session ID.
func LaunchArgv(toAgent, sameAgentID string) []string {
	if sameAgentID != "" && !nativeIDRe.MatchString(sameAgentID) {
		return nil
	}
	base := BaseAgent(toAgent)
	bin := toAgent
	if idx := strings.Index(bin, "@"); idx >= 0 {
		bin = bin[:idx]
	}
	switch base {
	case "claude":
		if sameAgentID != "" {
			return []string{bin, "--resume", sameAgentID}
		}
		return []string{bin, HandoffPrompt}
	case "codex":
		if sameAgentID != "" {
			return []string{bin, "resume", sameAgentID}
		}
		return []string{bin, HandoffPrompt}
	case "agy":
		// `antigravity` is the IDE; the agent CLI is `agy`
		if bin == "antigravity" {
			bin = "agy"
		}
		if sameAgentID != "" {
			return []string{bin, "--conversation", sameAgentID}
		}
		return []string{bin, "-i", HandoffPrompt}
	}
	return nil
}

// FreshArgv starts an empty session in the target CLI (no handoff prompt). Nil for an unknown agent.
func FreshArgv(toAgent string) []string {
	base := BaseAgent(toAgent)
	bin := toAgent
	if idx := strings.Index(bin, "@"); idx >= 0 {
		bin = bin[:idx]
	}
	switch base {
	case "claude", "codex":
		return []string{bin}
	case "agy":
		if bin == "antigravity" {
			bin = "agy"
		}
		return []string{bin}
	}
	return nil
}

// ShellJoin quotes argv for a POSIX shell.
func ShellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if shellSafeRe.MatchString(a) {
			parts[i] = a
		} else {
			parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(parts, " ")
}

// ResumeCommand is what the user runs to continue in the target CLI.
func ResumeCommand(toAgent, sameAgentID string) string {
	return ShellJoin(LaunchArgv(toAgent, sameAgentID))
}

// GUIURL is the web GUI of an agent, or "" when it only has a desktop app (agy).
func GUIURL(agent string) string {
	switch agent {
	case "claude":
		return "https://claude.ai/code"
	case "codex":
		return "https://chatgpt.com/codex"
	}
	return ""
}

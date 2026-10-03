package session

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// AntigravityConversation represents a conversation from Antigravity CLI's SQLite db.
type AntigravityConversation struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ProjectName  string `json:"project_name"`
	WorkspaceURI string `json:"workspace_uri"`
	LastModified string `json:"last_modified"`
	RelativeTime string `json:"relative_time"`
}

// ProjectGroup groups conversations by workspace/project.
type ProjectGroup struct {
	Name          string                    `json:"name"`
	Conversations []AntigravityConversation `json:"conversations"`
}

// LoadAntigravityProjects reads ~/.gemini/antigravity-cli/conversation_summaries.db
func LoadAntigravityProjects() ([]ProjectGroup, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(home, ".gemini/antigravity-cli/conversation_summaries.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("antigravity database not found at %s", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open antigravity db: %w", err)
	}
	defer db.Close()

	query := `
		SELECT conversation_id, title, workspace_uris, last_modified_time 
		FROM conversation_summaries 
		WHERE title != '' 
		ORDER BY last_modified_time DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query antigravity summaries: %w", err)
	}
	defer rows.Close()

	projectMap := make(map[string][]AntigravityConversation)

	for rows.Next() {
		var id, title, uris, lastModStr string
		if err := rows.Scan(&id, &title, &uris, &lastModStr); err != nil {
			continue
		}

		projectName := parseProjectName(uris)
		relTime := formatRelativeTime(lastModStr)

		c := AntigravityConversation{
			ID:           id,
			Title:        title,
			ProjectName:  projectName,
			WorkspaceURI: uris,
			LastModified: lastModStr,
			RelativeTime: relTime,
		}

		projectMap[projectName] = append(projectMap[projectName], c)
	}

	// Chuyển map thành slice ProjectGroup
	var groups []ProjectGroup
	// Ưu tiên AI Workspace lên đầu
	if convs, ok := projectMap["AI Workspace"]; ok {
		groups = append(groups, ProjectGroup{Name: "AI Workspace", Conversations: convs})
		delete(projectMap, "AI Workspace")
	}

	for pName, convs := range projectMap {
		groups = append(groups, ProjectGroup{Name: pName, Conversations: convs})
	}

	return groups, nil
}

// ReadAntigravityTranscript reads messages from transcript.jsonl
func ReadAntigravityTranscript(conversationID string) ([]Message, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	transcriptPath := filepath.Join(home, ".gemini/antigravity-cli/brain", conversationID, ".system_generated/logs/transcript.jsonl")
	file, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var messages []Message
	var currentSteps []ToolStep
	var turnStartTime time.Time

	scanner := bufio.NewScanner(file)
	// Cho phép đọc dòng JSON lớn
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var raw struct {
			Source    string `json:"source"`
			Type      string `json:"type"`
			Content   string `json:"content"`
			CreatedAt string `json:"created_at"`
			Media     []struct {
				MimeType string `json:"mime_type"`
				URI      string `json:"uri"`
			} `json:"media"`
			ToolCalls []struct {
				Name string         `json:"name"`
				Args map[string]any `json:"args"`
			} `json:"tool_calls"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		if raw.Type == "USER_INPUT" && (raw.Content != "" || len(raw.Media) > 0) {
			cleanContent := extractUserRequest(raw.Content)
			turnStartTime = parseTimestamp(raw.CreatedAt)

			var mediaItems []MediaItem
			for _, m := range raw.Media {
				mediaItems = append(mediaItems, MediaItem{
					MimeType: m.MimeType,
					URI:      m.URI,
				})
			}

			messages = append(messages, Message{
				ID:        fmt.Sprintf("%s-%d", conversationID, len(messages)),
				SessionID: conversationID,
				Role:      "user",
				Content:   cleanContent,
				Media:     mediaItems,
			})
			currentSteps = nil
		} else if raw.Type == "PLANNER_RESPONSE" {
			// Thu thập các Tool Call của turn hiện tại
			for _, tc := range raw.ToolCalls {
				action := cleanVal(tc.Args["toolAction"])
				summary := cleanVal(tc.Args["toolSummary"])
				cmd := cleanVal(tc.Args["CommandLine"])
				cwd := cleanVal(tc.Args["Cwd"])
				path := cleanVal(tc.Args["AbsolutePath"])
				if path == "" {
					path = cleanVal(tc.Args["TargetFile"])
				}

				if action == "" && summary == "" {
					if cmd != "" {
						action = "Ran command: " + cmd
					} else if path != "" {
						action = "Viewed " + filepath.Base(path)
					} else {
						action = tc.Name
					}
				}
				currentSteps = append(currentSteps, ToolStep{
					Name:    tc.Name,
					Action:  action,
					Summary: summary,
					Command: cmd,
					Cwd:     cwd,
					Path:    path,
					Status:  "done",
				})
			}

			if raw.Content != "" {
				var durStr string
				if !turnStartTime.IsZero() {
					turnEndTime := parseTimestamp(raw.CreatedAt)
					if !turnEndTime.IsZero() && turnEndTime.After(turnStartTime) {
						durSec := int(turnEndTime.Sub(turnStartTime).Seconds())
						if durSec > 0 {
							durStr = fmt.Sprintf("%ds", durSec)
						}
					}
				}

				messages = append(messages, Message{
					ID:        fmt.Sprintf("%s-%d", conversationID, len(messages)),
					SessionID: conversationID,
					Role:      "assistant",
					Content:   raw.Content,
					Agent:     "agy",
					Model:     "Gemini",
					Duration:  durStr,
					Steps:     currentSteps,
					IsRunning: false,
				})
				currentSteps = nil
			}
		} else if raw.Type == "GENERIC" && len(currentSteps) > 0 {
			// Gắn output vào ToolStep gần nhất chưa có output
			cleanedOutput := cleanCommandOutput(raw.Content)
			for i := len(currentSteps) - 1; i >= 0; i-- {
				if currentSteps[i].Output == "" {
					currentSteps[i].Output = cleanedOutput
					if strings.Contains(raw.Content, "The command exited with code ") &&
						!strings.Contains(raw.Content, "The command exited with code 0") {
						currentSteps[i].Status = "error"
					} else {
						currentSteps[i].Status = "done"
					}
					break
				}
			}
		}
	}

	// Nếu turn cuối cùng vẫn đang chạy dở (User đã hỏi nhưng Assistant chưa hoàn thành Content)
	if len(messages) > 0 && messages[len(messages)-1].Role == "user" {
		messages = append(messages, Message{
			ID:        fmt.Sprintf("%s-%d", conversationID, len(messages)),
			SessionID: conversationID,
			Role:      "assistant",
			Content:   "",
			Agent:     "agy",
			Model:     "Gemini",
			Steps:     currentSteps,
			IsRunning: true,
		})
	}

	return messages, nil
}

func cleanVal(val any) string {
	if val == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprintf("%v", val))
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") && len(s) >= 2 {
		if unquoted, err := strconv.Unquote(s); err == nil {
			s = unquoted
		} else {
			s = strings.Trim(s, "\"")
		}
	}
	return s
}

func parseProjectName(uris string) string {
	var list []string
	if err := json.Unmarshal([]byte(uris), &list); err == nil && len(list) > 0 {
		u, err := url.Parse(list[0])
		if err == nil {
			base := filepath.Base(u.Path)
			if base != "" && base != "." && base != "/" {
				return base
			}
		}
	}
	return "CLI Project"
}

func formatRelativeTime(timeStr string) string {
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05.999999999-07:00", timeStr)
		if err != nil {
			return ""
		}
	}

	diff := time.Since(t)
	if diff < time.Minute {
		return "now"
	} else if diff < time.Hour {
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	} else if diff < 24*time.Hour {
		return fmt.Sprintf("%dh", int(diff.Hours()))
	} else {
		return fmt.Sprintf("%dd", int(diff.Hours()/24))
	}
}

func extractUserRequest(raw string) string {
	start := strings.Index(raw, "<USER_REQUEST>")
	end := strings.Index(raw, "</USER_REQUEST>")
	if start != -1 && end != -1 && end > start {
		return strings.TrimSpace(raw[start+len("<USER_REQUEST>") : end])
	}
	return raw
}

func parseTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", ts); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t
	}
	return time.Time{}
}

func cleanCommandOutput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// 1. Nếu có format Output:\n
	if idx := strings.Index(s, "Output:\n"); idx != -1 {
		return strings.TrimSpace(s[idx+len("Output:\n"):])
	}

	// 2. Nếu có format Stdout:
	if idx := strings.Index(s, "Stdout:"); idx != -1 {
		out := strings.TrimSpace(s[idx+len("Stdout:"):])
		if errIdx := strings.Index(out, "Stderr:"); errIdx != -1 {
			stdout := strings.TrimSpace(out[:errIdx])
			stderr := strings.TrimSpace(out[errIdx+len("Stderr:"):])
			if stdout != "" && stderr != "" {
				return stdout + "\n" + stderr
			} else if stderr != "" {
				return stderr
			}
			return stdout
		}
		return out
	}

	// 3. Nếu là log có header Created At / Completed At / The command exited with code...
	lines := strings.Split(s, "\n")
	var filtered []string
	skipHeader := true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if skipHeader {
			if strings.HasPrefix(trimmed, "Created At:") ||
				strings.HasPrefix(trimmed, "Completed At:") ||
				strings.HasPrefix(trimmed, "The command exited with code") ||
				trimmed == "" {
				continue
			}
			skipHeader = false
		}
		filtered = append(filtered, line)
	}

	if len(filtered) > 0 {
		return strings.TrimSpace(strings.Join(filtered, "\n"))
	}

	return s
}

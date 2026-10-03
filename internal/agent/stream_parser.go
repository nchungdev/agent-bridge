package agent

import (
	"encoding/json"
	"regexp"
	"strings"
)

// StreamParser converts raw CLI output (either NDJSON from stream-json or raw text)
// into structured StreamEvent objects.
type StreamParser struct {
	inDiffBlock bool
	diffFile    string
	diffLines   strings.Builder

	inToolBlock bool
	toolCommand string
	toolOutput  strings.Builder
}

func NewStreamParser() *StreamParser {
	return &StreamParser{}
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\].*?\x07|\x1b\[.*?[mGKHJP]`)

// Parse takes a chunk of output (usually a line or buffer) and returns structured events.
func (p *StreamParser) Parse(chunk string) []StreamEvent {
	var events []StreamEvent

	lines := strings.Split(chunk, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		// 1. Thử parse theo cấu trúc JSON của agy stream-json
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			var parsedJSON struct {
				Event          string `json:"event"`
				ConversationID string `json:"conversation_id"`
				StepUpdate     *struct {
					State     string `json:"state"`
					StepType  string `json:"step_type"`
					TextDelta string `json:"text_delta"`
				} `json:"step_update"`
				Result *struct {
					Status   string `json:"status"`
					Response string `json:"response"`
				} `json:"result"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			}

			if err := json.Unmarshal([]byte(line), &parsedJSON); err == nil {
				// Event: agy khởi tạo conversation mới (nếu có ConversationID)
				if (parsedJSON.Event == "init" || parsedJSON.Event == "step_update") && parsedJSON.ConversationID != "" {
					events = append(events, StreamEvent{
						Type:      "session_created",
						SessionID: parsedJSON.ConversationID,
					})
				}

				// Event: Token stream delta
				if parsedJSON.StepUpdate != nil && parsedJSON.StepUpdate.TextDelta != "" {
					events = append(events, StreamEvent{
						Type:    "token",
						Content: parsedJSON.StepUpdate.TextDelta,
					})
					continue
				}

				// Event: Turn hoàn thành
				if parsedJSON.Event == "result" {
					if parsedJSON.Result != nil && parsedJSON.Result.Response != "" {
						events = append(events, StreamEvent{
							Type:    "token",
							Content: parsedJSON.Result.Response,
						})
					}
					events = append(events, StreamEvent{
						Type: "task_finished",
					})
					events = append(events, StreamEvent{
						Type: "done",
					})
					continue
				}

				// Event: Permission / Approval request (agy stream-json or tool approval)
				if parsedJSON.Event == "permission_request" || parsedJSON.Event == "ask_permission" || (parsedJSON.StepUpdate != nil && parsedJSON.StepUpdate.StepType == "permission_request") {
					cmdMsg := "Execute tool / command"
					if parsedJSON.Error != nil && parsedJSON.Error.Message != "" {
						cmdMsg = parsedJSON.Error.Message
					}
					events = append(events, StreamEvent{
						Type:             "tool_request",
						Tool:             "permission",
						Command:          cmdMsg,
						RequiresApproval: true,
					})
					continue
				}

				// Event: Error
				if parsedJSON.Error != nil {
					events = append(events, StreamEvent{
						Type:    "error",
						Message: parsedJSON.Error.Message,
					})
					continue
				}

				// Mọi event JSON khác (init, step_update không có text_delta, heartbeat...)
				// đều BỎ QUA để không bị lọt raw JSON ra ngoài làm text cho người dùng
				continue
			}
		}

		// 2. Fallback parse text thông thường nếu không phải stream-json
		clean := ansiRegex.ReplaceAllString(line, "")
		trimmed := strings.TrimSpace(clean)

		// Detect diff blocks
		if strings.HasPrefix(trimmed, "--- a/") || (strings.HasPrefix(trimmed, "--- ") && strings.Contains(trimmed, "/")) {
			p.inDiffBlock = true
			p.diffLines.Reset()
			p.diffLines.WriteString(line + "\n")
			continue
		}
		if p.inDiffBlock && strings.HasPrefix(trimmed, "+++ b/") {
			p.diffFile = strings.TrimPrefix(trimmed, "+++ b/")
			p.diffLines.WriteString(line + "\n")
			continue
		}
		if p.inDiffBlock && strings.HasPrefix(trimmed, "+++ ") {
			p.diffFile = strings.TrimPrefix(trimmed, "+++ ")
			p.diffLines.WriteString(line + "\n")
			continue
		}
		if p.inDiffBlock && (strings.HasPrefix(trimmed, "@@") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-")) {
			p.diffLines.WriteString(line + "\n")
			continue
		}
		if p.inDiffBlock {
			events = append(events, StreamEvent{
				Type:  "diff",
				File:  p.diffFile,
				Patch: p.diffLines.String(),
			})
			p.inDiffBlock = false
			p.diffFile = ""
			p.diffLines.Reset()
		}

		// Detect tool request
		if isToolRequest(trimmed) {
			cmd := extractCommand(trimmed)
			events = append(events, StreamEvent{
				Type:             "tool_request",
				Tool:             "bash",
				Command:          cmd,
				RequiresApproval: true,
			})
			continue
		}

		// Detect quota error
		if isQuotaError(trimmed) {
			events = append(events, StreamEvent{
				Type:    "error",
				Message: trimmed,
				Reason:  "quota_exceeded",
			})
			continue
		}

		// Regular text fallback
		events = append(events, StreamEvent{
			Type:    "token",
			Content: clean + "\n",
		})
	}

	return events
}

func isToolRequest(line string) bool {
	patterns := []string{
		"Running: ",
		"Execute: ",
		"$ ",
		"Command: ",
		"run_command",
		"bash(",
	}
	for _, p := range patterns {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

func extractCommand(line string) string {
	prefixes := []string{"Running: ", "Execute: ", "$ ", "Command: "}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return strings.TrimPrefix(line, p)
		}
	}
	return line
}

func isQuotaError(line string) bool {
	lower := strings.ToLower(line)
	patterns := []string{
		"429",
		"rate limit",
		"rate_limit",
		"quota exceeded",
		"quota_exceeded",
		"too many requests",
		"resource_exhausted",
		"daily limit",
		"usage limit",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

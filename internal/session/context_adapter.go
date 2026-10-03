package session

import (
	"fmt"
	"strings"
)

// ContextAdapter converts the agnostic session messages into a format
// suitable for each CLI agent's stdin/prompt injection.
type ContextAdapter struct{}

func NewContextAdapter() *ContextAdapter {
	return &ContextAdapter{}
}

// ForAGY formats context for Gemini CLI / Antigravity.
// AGY accepts a prompt via stdin when run non-interactively with -p flag.
// We format as a markdown conversation summary.
func (a *ContextAdapter) ForAGY(messages []Message, newPrompt string) string {
	if len(messages) == 0 {
		return newPrompt
	}

	var b strings.Builder
	b.WriteString("<context>\n")
	b.WriteString("Below is the conversation history from a prior session. Continue from where we left off.\n\n")

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			b.WriteString(fmt.Sprintf("**User**: %s\n\n", truncate(msg.Content, 2000)))
		case "assistant":
			agent := msg.Agent
			if agent == "" {
				agent = "AI"
			}
			b.WriteString(fmt.Sprintf("**%s** (model: %s): %s\n\n", agent, msg.Model, truncate(msg.Content, 3000)))
		}
	}
	b.WriteString("</context>\n\n")
	b.WriteString("Now, please handle this new request:\n\n")
	b.WriteString(newPrompt)
	return b.String()
}

// ForClaude formats context for Claude Code CLI.
// Claude Code uses --resume flag to continue sessions, or accepts prompt via -p.
// We inject context as a conversation transcript.
func (a *ContextAdapter) ForClaude(messages []Message, newPrompt string) string {
	if len(messages) == 0 {
		return newPrompt
	}

	var b strings.Builder
	b.WriteString("Previous conversation context:\n\n")

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			b.WriteString(fmt.Sprintf("H: %s\n\n", truncate(msg.Content, 2000)))
		case "assistant":
			b.WriteString(fmt.Sprintf("A: %s\n\n", truncate(msg.Content, 3000)))
		}
	}

	b.WriteString("---\n\n")
	b.WriteString("New request:\n\n")
	b.WriteString(newPrompt)
	return b.String()
}

// ForCodex formats context for OpenAI Codex CLI.
func (a *ContextAdapter) ForCodex(messages []Message, newPrompt string) string {
	// Codex CLI is similar to Claude in accepting prompt via stdin
	return a.ForClaude(messages, newPrompt)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}

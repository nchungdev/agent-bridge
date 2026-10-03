package manager

import (
	"regexp"

	"github.com/nchungdev/agent-hub/internal/core"
)

type Verdict int

const (
	Ask Verdict = iota
	Allow
	Deny
	// AskAlways always shows the request to the user: no mode, policy or
	// "allow for this chat" shortcut may auto-approve it (privilege escalation).
	AskAlways
)

var privilegedRe = regexp.MustCompile(`(^|[\s;&|(\x60])(sudo|su|doas|pkexec)([\s;&|)]|$)`)

// IsPrivileged reports whether a command line escalates privileges.
func IsPrivileged(cmd string) bool { return privilegedRe.MatchString(cmd) }

// Policy decides whether an approval request is auto-resolved or shown to the user.
// It is engine-agnostic: rules are written once for every adapter.
type Policy interface {
	Evaluate(mode string, r core.ApprovalRequest) Verdict
}

// DefaultPolicy: bypass allows everything (explicit, per-session opt-in), plan
// denies anything that is not read-only, otherwise read-only tools are allowed
// and the rest are shown to the user.
type DefaultPolicy struct{ ReadOnly, Edits map[string]bool }

func NewDefaultPolicy() *DefaultPolicy {
	return &DefaultPolicy{ReadOnly: map[string]bool{"Read": true, "Grep": true, "Glob": true, "LS": true, "read_file": true, "search": true},
		Edits: map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true, "FileChange": true}}
}

func (p *DefaultPolicy) Evaluate(mode string, r core.ApprovalRequest) Verdict {
	ro := p.ReadOnly[r.Tool]
	if cmd, _ := r.Args["command"].(string); cmd != "" && IsPrivileged(cmd) {
		if mode == "plan" {
			return Deny
		}
		return AskAlways
	}
	switch mode {
	case "bypass":
		return Allow
	case "plan":
		if ro {
			return Allow
		}
		return Deny
	case "accept-edits":
		if ro || p.Edits[r.Tool] {
			return Allow
		}
		return Ask
	}
	if ro {
		return Allow
	}
	return Ask
}

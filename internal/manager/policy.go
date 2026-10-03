package manager

import (
	"regexp"
	"strings"

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
	case "auto":
		// like accept-edits, plus commands that are provably read-only; everything else still asks
		if ro || p.Edits[r.Tool] {
			return Allow
		}
		if cmd, _ := r.Args["command"].(string); cmd != "" && SafeCommand(cmd) {
			return Allow
		}
		return Ask
	}
	if ro {
		return Allow
	}
	return Ask
}

// safeCommands are programs that only read. Pipes between them are fine; anything that can write,
// chain, substitute or run another program is not.
var safeCommands = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "grep": true, "egrep": true, "fgrep": true, "rg": true,
	"wc": true, "pwd": true, "echo": true, "which": true, "whoami": true, "date": true, "uname": true,
	"df": true, "du": true, "stat": true, "file": true, "tree": true, "basename": true, "dirname": true,
	"realpath": true, "sort": true, "uniq": true, "cut": true, "tr": true, "diff": true, "cmp": true,
	"md5sum": true, "sha256sum": true, "ps": true, "lsblk": true, "free": true, "uptime": true, "find": true, "git": true,
}

var safeGit = map[string]bool{"status": true, "diff": true, "log": true, "show": true, "branch": true, "rev-parse": true, "ls-files": true, "blame": true, "describe": true, "remote": true}

var unsafeShell = regexp.MustCompile("[;&<>`\\n]|\\$\\(|\\$\\{")

// SafeCommand reports whether a shell command line is made only of read-only programs.
func SafeCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" || unsafeShell.MatchString(cmd) || IsPrivileged(cmd) {
		return false
	}
	for _, seg := range strings.Split(cmd, "|") { // "||" was already rejected by the & check? no: handle explicitly
		f := strings.Fields(seg)
		if len(f) == 0 {
			return false
		}
		prog := f[0]
		if !safeCommands[prog] {
			return false
		}
		switch prog {
		case "find":
			for _, a := range f[1:] {
				if a == "-exec" || a == "-execdir" || a == "-ok" || a == "-okdir" || a == "-delete" || strings.HasPrefix(a, "-fprint") || a == "-fls" {
					return false
				}
			}
		case "git":
			if len(f) < 2 || !safeGit[f[1]] {
				return false
			}
			for _, a := range f[2:] { // branch -d/-D/-m, remote add/remove...
				if prog == "git" && f[1] == "branch" && (a == "-d" || a == "-D" || a == "-m" || a == "-M" || a == "-c" || a == "-C") {
					return false
				}
				if f[1] == "remote" && (a == "add" || a == "remove" || a == "rm" || a == "rename" || a == "set-url" || a == "prune") {
					return false
				}
			}
		case "sort":
			for _, a := range f[1:] {
				if a == "-o" || strings.HasPrefix(a, "--output") {
					return false
				}
			}
		}
	}
	return !strings.Contains(cmd, "||")
}

package bridge

import (
	"strings"
	"unicode"
)

// Naming a conversation after its task. Every agent CLI does it differently, so each one is a row here:
//   - flag:  the CLI takes the name at launch (`claude --name <title>`)
//   - slash: the CLI has a slash command that renames the running conversation (`/rename <title>`); the server
//     types it once the CLI is up, and only on a resumed session (agy: "no active conversation" before the first
//     message)
//
// A new agent CLI gets its naming by adding a row; one without a row is simply left unnamed.
type namingSpec struct{ flag, slash string }

var naming = map[string]namingSpec{
	"claude": {flag: "--name"},
	"agy":    {slash: "/rename"},
	"codex":  {slash: "/rename"},
}

// cleanTitle is a task title made safe to pass as one argument; "" when there is nothing worth naming.
func cleanTitle(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > 60 {
		s = string(r[:60])
	}
	if s == "New Task" { // the placeholder of a task nobody named yet
		return ""
	}
	return s
}

// NameFlagArgs is the launch argument pair that names the conversation, nil when the agent has no such flag.
func NameFlagArgs(agent, title string) []string {
	t := cleanTitle(title)
	if f := naming[BaseAgent(agent)].flag; f != "" && t != "" {
		return []string{f, t}
	}
	return nil
}

// RenameInput is the line to type into a running session to name it ("" when the agent has no rename command).
func RenameInput(agent, title string) string {
	t := cleanTitle(title)
	if c := naming[BaseAgent(agent)].slash; c != "" && t != "" {
		return c + " " + t + "\n"
	}
	return ""
}

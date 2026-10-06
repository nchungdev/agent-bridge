package bridge

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoveredEngine represents an installed CLI agent or variant found on the host.
type DiscoveredEngine struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Base       string `json:"base"`
	Binary     string `json:"binary"`
	Installed  bool   `json:"installed"`
	InstallCmd string `json:"install_cmd,omitempty"`
}

// DiscoverEngines scans PATH to find primary agents (agy, claude, codex)
// and any auto-discovered variants like claude-me, claude-work, agy-personal, codex-team, etc.
func DiscoverEngines() []DiscoveredEngine {
	type candidate struct {
		id         string
		name       string
		base       string
		binary     string
		installCmd string
	}

	// Default baseline engines
	standards := []candidate{
		{id: "agy", name: "Antigravity", base: "agy", binary: "antigravity", installCmd: ""},
		{id: "claude", name: "Claude Code", base: "claude", binary: "claude", installCmd: "npm install -g @anthropic-ai/claude-code"},
		{id: "codex", name: "Codex", base: "codex", binary: "codex", installCmd: "npm install -g @openai/codex"},
	}

	seen := make(map[string]bool)
	var result []DiscoveredEngine

	// 1. Check standard engines
	for _, std := range standards {
		installed := isBinaryAvailable(std.binary)
		if !installed && std.id == "agy" {
			installed = isBinaryAvailable("agy")
		}
		result = append(result, DiscoveredEngine{
			ID:         std.id,
			Name:       std.name,
			Base:       std.base,
			Binary:     std.binary,
			Installed:  installed,
			InstallCmd: std.installCmd,
		})
		seen[std.id] = true
		if std.binary != "" {
			seen[std.binary] = true
		}
		if std.id == "agy" {
			seen["agy"] = true
			seen["antigravity"] = true
		}
	}

	// 2. Scan PATH for variants matching claude-*, agy-*, codex-*
	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	discoveredVariants := make(map[string]candidate)

	for _, dir := range pathDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if seen[name] || discoveredVariants[name] != (candidate{}) {
				continue
			}

			var base, displayName string
			if strings.HasPrefix(name, "claude-") || strings.HasPrefix(name, "claude_") {
				base = "claude"
				suffix := name[7:]
				displayName = "Claude (" + suffix + ")"
			} else if strings.HasPrefix(name, "agy-") || strings.HasPrefix(name, "agy_") {
				base = "agy"
				suffix := name[4:]
				displayName = "Antigravity (" + suffix + ")"
			} else if strings.HasPrefix(name, "codex-") || strings.HasPrefix(name, "codex_") {
				base = "codex"
				suffix := name[6:]
				displayName = "Codex (" + suffix + ")"
			} else {
				continue
			}

			// Ensure file is executable
			info, err := entry.Info()
			if err != nil || (info.Mode()&0111) == 0 {
				continue
			}

			discoveredVariants[name] = candidate{
				id:     name,
				name:   displayName,
				base:   base,
				binary: name,
			}
		}
	}

	// Sort variants by ID
	var variantKeys []string
	for k := range discoveredVariants {
		variantKeys = append(variantKeys, k)
	}
	sort.Strings(variantKeys)

	for _, k := range variantKeys {
		v := discoveredVariants[k]
		result = append(result, DiscoveredEngine{
			ID:        v.id,
			Name:      v.name,
			Base:      v.base,
			Binary:    v.id,
			Installed: true,
		})
	}

	return result
}

func isBinaryAvailable(name string) bool {
	if name == "" {
		return false
	}
	if strings.Contains(name, "/") {
		info, err := os.Stat(name)
		return err == nil && !info.IsDir() && (info.Mode()&0111) != 0
	}
	_, err := exec.LookPath(name)
	return err == nil
}

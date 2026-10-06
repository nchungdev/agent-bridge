package bridge

import (
	"os"
	"path/filepath"
)

// DefaultWorkspacePath returns a sensible default workspace directory across platforms.
// Resolution order:
// 1. Environment variable AGENT_BRIDGE_DEFAULT_WORKSPACE or AGENT_BRIDGE_WORKSPACE
// 2. First existing subdirectory in user home: "AI Workspace", "Projects", "workspace", "Developer"
// 3. User home directory
// 4. Current working directory
// 5. Current directory "."
func DefaultWorkspacePath() string {
	if env := os.Getenv("AGENT_BRIDGE_DEFAULT_WORKSPACE"); env != "" {
		if st, err := os.Stat(env); err == nil && st.IsDir() {
			return filepath.Clean(env)
		}
	}
	if env := os.Getenv("AGENT_BRIDGE_WORKSPACE"); env != "" {
		if st, err := os.Stat(env); err == nil && st.IsDir() {
			return filepath.Clean(env)
		}
	}

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		candidates := []string{"AI Workspace", "Projects", "workspace", "Developer"}
		for _, c := range candidates {
			p := filepath.Join(home, c)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				return p
			}
		}
		return home
	}

	if wd, err := os.Getwd(); err == nil && wd != "" {
		return wd
	}

	return "."
}

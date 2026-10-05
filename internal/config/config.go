package config

import (
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Port    int
	DataDir string
	Agents  map[string]AgentConfig
}

type AgentConfig struct {
	BinaryPath string
	EnvVars    map[string]string
}

func Load() *Config {
	port := 8080
	if p := os.Getenv("AGENT_BRIDGE_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	dataDir := os.Getenv("AGENT_BRIDGE_DATA_DIR")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".agent-bridge")
	}
	os.MkdirAll(dataDir, 0755)

	agents := map[string]AgentConfig{
		"agy": {
			BinaryPath: findBinary("antigravity", "/usr/bin/antigravity"),
		},
		"claude": {
			BinaryPath: findBinary("claude", ""),
		},
		"codex": {
			BinaryPath: findBinary("codex", ""),
		},
	}

	return &Config{
		Port:    port,
		DataDir: dataDir,
		Agents:  agents,
	}
}

func findBinary(name, defaultPath string) string {
	if defaultPath != "" {
		if _, err := os.Stat(defaultPath); err == nil {
			return defaultPath
		}
	}
	// Search in PATH
	if path, err := lookPath(name); err == nil {
		return path
	}
	return ""
}

func lookPath(name string) (string, error) {
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, nil
		}
	}
	return "", os.ErrNotExist
}

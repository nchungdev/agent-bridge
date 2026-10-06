package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVariantsLaunchArgv(t *testing.T) {
	cases := []struct {
		agent, id, want string
	}{
		{"claude-me", "", "claude-me 'Đọc .agent/handoff.md và tiếp tục công việc dang dở.'"},
		{"claude-me", "3f2a-b1", "claude-me --resume 3f2a-b1"},
		{"codex-work", "abc", "codex-work resume abc"},
		{"agy-custom", "", "agy-custom -i 'Đọc .agent/handoff.md và tiếp tục công việc dang dở.'"},
		{"agy-custom", "15cee76f", "agy-custom --conversation 15cee76f"},
	}
	for _, c := range cases {
		if got := ResumeCommand(c.agent, c.id); got != c.want {
			t.Errorf("ResumeCommand(%q,%q) = %q, want %q", c.agent, c.id, got, c.want)
		}
	}
}

func TestVariantsRemoteModeAndBuildArgv(t *testing.T) {
	if RemoteModeOf("claude-me") != RemotePerSession {
		t.Errorf("expected RemotePerSession for claude-me, got %v", RemoteModeOf("claude-me"))
	}
	if RemoteModeOf("agy-personal") != RemoteBoth {
		t.Errorf("expected RemoteBoth for agy-personal, got %v", RemoteModeOf("agy-personal"))
	}
	if RemoteModeOf("codex-team") != RemoteDaemon {
		t.Errorf("expected RemoteDaemon for codex-team, got %v", RemoteModeOf("codex-team"))
	}

	cmd := ShellJoin(BuildArgv("claude-me", LaunchOpts{Fresh: true, Remote: true}))
	if cmd != "claude-me --remote-control" {
		t.Errorf("BuildArgv(claude-me) = %q, want %q", cmd, "claude-me --remote-control")
	}
}

func TestIsProtectedWorkspacePath(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		path      string
		protected bool
	}{
		{"/", true},
		{"", true},
		{".", true},
		{"~", true},
		{"/root", true},
		{"/home", true},
		{"/Users", true},
		{"/tmp", true},
		{"/home/developer", true},
		{"/Users/developer", true},
		{home, true},
		{"/home/developer/workspace", false},
		{"/Users/developer/Documents/Project", false},
		{"/var/www/myproject", false},
	}

	for _, tc := range tests {
		got := IsProtectedWorkspacePath(tc.path)
		if got != tc.protected {
			t.Errorf("IsProtectedWorkspacePath(%q) = %v, want %v", tc.path, got, tc.protected)
		}
	}
}

func TestDiscoverEngines(t *testing.T) {
	// Create a temporary directory in PATH with a mock executable `claude-custom`
	tmpDir, err := os.MkdirTemp("", "eng-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mockBin := filepath.Join(tmpDir, "claude-custom")
	if err := os.WriteFile(mockBin, []byte("#!/bin/sh\necho custom"), 0755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(filepath.ListSeparator)+origPath)
	defer os.Setenv("PATH", origPath)

	engines := DiscoverEngines()
	found := false
	for _, e := range engines {
		if e.ID == "claude-custom" {
			found = true
			if e.Base != "claude" {
				t.Errorf("expected Base to be claude, got %s", e.Base)
			}
			if e.Name != "Claude (custom)" {
				t.Errorf("expected Name to be 'Claude (custom)', got %s", e.Name)
			}
			if !e.Installed {
				t.Errorf("expected Installed to be true")
			}
			break
		}
	}
	if !found {
		t.Errorf("claude-custom was not discovered in PATH")
	}
}

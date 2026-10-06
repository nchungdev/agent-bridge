package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchArgv(t *testing.T) {
	cases := []struct {
		agent, id, want string
	}{
		{"claude", "", "claude 'Đọc .agent/handoff.md và tiếp tục công việc dang dở.'"},
		{"claude", "3f2a-b1", "claude --resume 3f2a-b1"},
		{"codex", "abc", "codex resume abc"},
		{"agy", "", "agy -i 'Đọc .agent/handoff.md và tiếp tục công việc dang dở.'"},
		{"agy", "15cee76f", "agy --conversation 15cee76f"},
	}
	for _, c := range cases {
		if got := ResumeCommand(c.agent, c.id); got != c.want {
			t.Errorf("ResumeCommand(%q,%q) = %q, want %q", c.agent, c.id, got, c.want)
		}
	}
	if LaunchArgv("unknown", "") != nil {
		t.Error("unknown agent should give nil argv")
	}
	if LaunchArgv("claude", "x; rm -rf ~") != nil {
		t.Error("malformed session id should give nil argv")
	}
}

func TestFreshArgv(t *testing.T) {
	for _, a := range []string{"claude", "codex", "agy"} {
		if got := FreshArgv(a); len(got) != 1 || got[0] != a {
			t.Errorf("FreshArgv(%q) = %v", a, got)
		}
	}
	if FreshArgv("nope") != nil {
		t.Error("unknown agent should give nil")
	}
}

func TestShellJoinQuotes(t *testing.T) {
	if got := ShellJoin([]string{"echo", "it's"}); got != `echo 'it'\''s'` {
		t.Errorf("got %q", got)
	}
}

func TestBuildArgvRemote(t *testing.T) {
	cases := []struct {
		agent string
		o     LaunchOpts
		want  string
	}{
		{"claude", LaunchOpts{Fresh: true, Remote: true}, "claude --remote-control"},
		{"claude", LaunchOpts{ID: "abc", Remote: true}, "claude --resume abc --remote-control"},
		{"claude", LaunchOpts{Remote: true, Name: "My Project"}, "claude --remote-control 'My Project' 'Đọc .agent/handoff.md và tiếp tục công việc dang dở.'"},
		{"claude", LaunchOpts{Remote: true}, "claude --remote-control 'Agent Bridge' 'Đọc .agent/handoff.md và tiếp tục công việc dang dở.'"},
		{"agy", LaunchOpts{Fresh: true, Remote: true}, "agy --remote-control"},
		{"agy", LaunchOpts{ID: "15cee76f", Remote: true}, "agy --remote-control --conversation 15cee76f"},
		{"agy", LaunchOpts{Remote: true}, "agy --remote-control -i 'Đọc .agent/handoff.md và tiếp tục công việc dang dở.'"},
		{"codex", LaunchOpts{Fresh: true, Remote: true}, "codex"}, // remote access is the shared daemon, no flag
		{"claude", LaunchOpts{Fresh: true}, "claude"},
		{"claude", LaunchOpts{Fresh: true, Title: "Giao diện chart"}, "claude --name 'Giao diện chart'"},
		{"claude", LaunchOpts{ID: "abc", Remote: true, Title: "T"}, "claude --name T --resume abc --remote-control"},
		{"claude", LaunchOpts{Fresh: true, Title: "New Task"}, "claude"},
		{"agy", LaunchOpts{Fresh: true, Title: "T"}, "agy"},
	}
	for _, c := range cases {
		if got := ShellJoin(BuildArgv(c.agent, c.o)); got != c.want {
			t.Errorf("BuildArgv(%s, %+v) = %q, want %q", c.agent, c.o, got, c.want)
		}
	}
	if BuildArgv("nope", LaunchOpts{Remote: true}) != nil || BuildArgv("claude", LaunchOpts{ID: "x; rm -rf ~", Remote: true}) != nil {
		t.Error("unknown agent or malformed id must give nil")
	}
}

func TestCleanRemoteName(t *testing.T) {
	if got := cleanRemoteName("a\nb\x1b[31m "); got != "ab[31m" {
		t.Errorf("control characters must be stripped, got %q", got)
	}
	if got := cleanRemoteName(""); got != "Agent Bridge" {
		t.Errorf("default = %q", got)
	}
	if got := cleanRemoteName(strings.Repeat("x", 100)); len([]rune(got)) != 60 {
		t.Errorf("length = %d, want 60", len([]rune(got)))
	}
}

func TestRemoteArgv(t *testing.T) {
	if got := ShellJoin(remoteArgv("codex", "pair")); got != "codex remote-control pair" {
		t.Errorf("got %q", got)
	}
	if remoteArgv("claude", "enable") != nil || remoteArgv("codex", "status") != nil || remoteArgv("agy", "pair") != nil {
		t.Error("actions an agent does not have must give nil")
	}
	if RemoteModeOf("claude") != RemotePerSession || RemoteModeOf("codex") != RemoteDaemon || RemoteModeOf("x") != "" {
		t.Error("wrong remote modes")
	}
}

func TestContextUsageFindsSessionOutsideGivenFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude", "projects", "-home-user-proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "8fa8e160-cd1c-47b7-8510-41ab4ad87d37"
	line := `{"type":"assistant","message":{"model":"claude-x","usage":{"input_tokens":10,"cache_creation_input_tokens":90,"cache_read_input_tokens":400}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manager{}
	// asked for a different folder than the one the session was recorded under
	got := m.GetContextUsage("claude", id, "/home/user/proj/sub/dir")
	if !got.Supported || got.Tokens != 500 || got.Window != 200000 || got.Model != "claude-x" {
		t.Errorf("sub-folder lookup = %+v", got)
	}
	if got := m.GetContextUsage("claude", id, ""); !got.Supported {
		t.Errorf("empty folder lookup = %+v", got)
	}
	if got := m.GetContextUsage("claude", "../../etc/passwd", "/x"); got.Supported {
		t.Error("an id with path separators must be rejected")
	}
}

package bridge

import "testing"

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

func TestShellJoinQuotes(t *testing.T) {
	if got := ShellJoin([]string{"echo", "it's"}); got != `echo 'it'\''s'` {
		t.Errorf("got %q", got)
	}
}

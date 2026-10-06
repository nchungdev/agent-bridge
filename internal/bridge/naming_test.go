package bridge

import "testing"

func TestNaming(t *testing.T) {
	if got := NameFlagArgs("claude", " Giao diện\nchart "); len(got) != 2 || got[0] != "--name" || got[1] != "Giao diệnchart" {
		t.Errorf("claude flag = %q", got)
	}
	if NameFlagArgs("claude", "New Task") != nil || NameFlagArgs("agy", "T") != nil || NameFlagArgs("nope", "T") != nil {
		t.Error("placeholder title, slash-only and unknown agents must give no flag")
	}
	if got := RenameInput("agy-personal", "T"); got != "/rename T\n" {
		t.Errorf("agy variant rename = %q", got)
	}
	if RenameInput("claude", "T") != "" || RenameInput("nope", "T") != "" || RenameInput("codex", "") != "" {
		t.Error("flag-only, unknown agents and empty titles must give no rename line")
	}
}

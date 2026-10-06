package accounts

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSigninState(t *testing.T) {
	home := t.TempDir()
	a := &AGYProfiles{Home: home}
	live := filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token")
	snap := func(id string) string { return filepath.Join(home, ".gemini", "profiles", id, "antigravity-oauth-token") }

	// the active profile is judged by the live token, the others by their saved snapshot
	writeFile(t, live, `{"refresh_token":"x"}`)
	writeFile(t, snap("work"), `{"refresh_token":"y"}`)
	writeFile(t, snap("broken"), `not json`)
	writeFile(t, snap("empty"), `{}`)

	cases := []struct {
		id        string
		active    bool
		known, in bool
		why       string
	}{
		{"main", true, true, true, "active profile with a live token"},
		{"work", false, true, true, "inactive profile with a saved snapshot"},
		{"missing", false, true, false, "no snapshot at all"},
		{"broken", false, true, false, "snapshot is not JSON"},
		{"empty", false, true, false, "snapshot is an empty object"},
		{"../etc", false, false, false, "id with path separators is rejected"},
		{"", false, false, false, "empty id is rejected"},
	}
	for _, c := range cases {
		known, in := a.SigninState(c.id, c.active)
		if known != c.known || in != c.in {
			t.Errorf("%s: SigninState(%q, %v) = (%v, %v), want (%v, %v)", c.why, c.id, c.active, known, in, c.known, c.in)
		}
	}

	// the active profile without a live token is signed out, whatever its snapshot says
	if err := os.Remove(live); err != nil {
		t.Fatal(err)
	}
	if known, in := a.SigninState("main", true); !known || in {
		t.Errorf("active profile without a live token = (%v, %v), want (true, false)", known, in)
	}
}

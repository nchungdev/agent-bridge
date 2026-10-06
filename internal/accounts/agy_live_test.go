package accounts

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func jwt(email string) string {
	return "h." + base64.RawURLEncoding.EncodeToString([]byte(`{"email":"`+email+`"}`)) + ".s"
}

func writeAGY(t *testing.T, path, email string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"token":"t","id_token":"`+jwt(email)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAGYEffectiveFollowsLiveLoginAndSwitchIsNotSkipped(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".gemini")
	writeAGY(t, filepath.Join(root, "antigravity-cli", "antigravity-oauth-token"), "a@x.test")
	writeAGY(t, filepath.Join(root, "profiles", "a", "antigravity-oauth-token"), "a@x.test")
	writeAGY(t, filepath.Join(root, "profiles", "b", "antigravity-oauth-token"), "b@x.test")
	meta := `{"active_profile":"b","profiles":[{"id":"a","name":"A","email":"a@x.test"},{"id":"b","name":"B","email":"b@x.test"}]}`
	if err := os.WriteFile(filepath.Join(root, "profiles", "profiles.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &AGYProfiles{Home: home, Restart: func(context.Context) error { return nil }}
	got, _ := p.Effective()
	if got.Active != "a" {
		t.Fatalf("active = %q, want a (live login)", got.Active)
	}
	if err := p.Switch(context.Background(), "b"); err != nil { // metadata already says b: must still switch
		t.Fatal(err)
	}
	if e := p.LiveEmail(); e != "b@x.test" {
		t.Fatalf("live = %q after switch", e)
	}
}

func TestAGYCaptureAddsAndRefreshesProfile(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".gemini")
	writeAGY(t, filepath.Join(root, "antigravity-cli", "antigravity-oauth-token"), "New.User@x.test")
	p := &AGYProfiles{Home: home}
	prof, err := p.Capture()
	if err != nil || prof.Email != "new.user@x.test" || prof.ID != "new_user" {
		t.Fatalf("capture = %+v, %v", prof, err)
	}
	again, err := p.Capture()
	if err != nil || again.ID != prof.ID {
		t.Fatalf("second capture = %+v, %v", again, err)
	}
	l, _ := p.List()
	if len(l.Profiles) != 1 || l.Active != "new_user" {
		t.Fatalf("list = %+v", l)
	}
}

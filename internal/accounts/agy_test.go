package accounts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAGYProfilesSwitch(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[fail], func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, ".gemini")
			write := func(path, data string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			metadata := `{"active_profile":"default","profiles":[{"id":"default","name":"Default"},{"id":"work","name":"Work","created_at":"keep"}],"extra":"keep"}`
			write(filepath.Join(root, "profiles", "profiles.json"), metadata)
			token := filepath.Join(root, "antigravity-cli", "antigravity-oauth-token")
			write(token, `{"account":"old"}`)
			write(filepath.Join(root, "oauth_creds.json"), `{"account":"old"}`)
			write(filepath.Join(root, "profiles", "work", "antigravity-oauth-token"), `{"account":"new"}`)
			restarts := 0
			a := &AGYProfiles{Home: home, Restart: func(context.Context) error {
				restarts++
				if fail {
					return errors.New("failed")
				}
				return nil
			}}
			if err := a.Switch(context.Background(), "../work"); err == nil {
				t.Fatal("accepted traversal")
			}
			err := a.Switch(context.Background(), "work")
			if (err != nil) != fail {
				t.Fatalf("switch error: %v", err)
			}
			b, _ := os.ReadFile(token)
			want := "new"
			if fail {
				want = "old"
			}
			if !strings.Contains(string(b), want) {
				t.Fatalf("token not switched/rolled back: %s", b)
			}
			p, err := a.List()
			if err != nil {
				t.Fatal(err)
			}
			active := "work"
			if fail {
				active = "default"
			}
			if p.Active != active {
				t.Fatalf("active=%s", p.Active)
			}
			b, _ = os.ReadFile(a.path())
			if !strings.Contains(string(b), "created_at") || !strings.Contains(string(b), "extra") {
				t.Fatal("lost manager metadata")
			}
			if !fail {
				if _, err := os.Stat(filepath.Join(root, "oauth_creds.json")); !os.IsNotExist(err) {
					t.Fatal("retained old optional credentials")
				}
				if restarts != 1 {
					t.Fatalf("restarts=%d", restarts)
				}
			}
		})
	}
}

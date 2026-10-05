package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nchungdev/agent-hub/internal/accounts"
	"github.com/nchungdev/agent-hub/internal/core"
	"github.com/nchungdev/agent-hub/internal/enginetest"
	"github.com/nchungdev/agent-hub/internal/store"
)

func TestAGYAccountsUseManagerProfiles(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	root := filepath.Join(home, ".gemini")
	if err := os.MkdirAll(filepath.Join(root, "profiles", "work"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "antigravity-cli"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "profiles", "profiles.json"), []byte(`{"active_profile":"default","profiles":[{"id":"work","name":"Work","email":"work@example.test"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "profiles", "work", "antigravity-oauth-token"), []byte(`{"token":"test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := accounts.New(st, t.TempDir(), []core.Engine{enginetest.New("agy")})
	if err != nil {
		t.Fatal(err)
	}
	restarts := 0
	v := &V2{Registry: reg, AGYProfiles: &accounts.AGYProfiles{Home: home, Restart: func(context.Context) error { restarts++; return nil }}}
	w := httptest.NewRecorder()
	v.handleAccounts(w, httptest.NewRequest("GET", "/api/v2/accounts", nil))
	var groups []accountGroup
	if err := json.Unmarshal(w.Body.Bytes(), &groups); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Accounts) != 1 || groups[0].Accounts[0].ID != "agy@work" || groups[0].Accounts[0].CanLogin {
		t.Fatalf("unexpected profiles: %+v", groups)
	}
	w = httptest.NewRecorder()
	v.handleAccountActive(w, httptest.NewRequest("PUT", "/api/v2/accounts/active", strings.NewReader(`{"engine":"agy","id":"agy@work"}`)))
	if w.Code != 204 || restarts != 1 {
		t.Fatalf("switch status=%d body=%s restarts=%d", w.Code, w.Body.String(), restarts)
	}
	if reg.Active("agy") != "agy" || len(reg.Engines()) != 1 {
		t.Fatal("AGY profiles must keep the base engine identity")
	}
}

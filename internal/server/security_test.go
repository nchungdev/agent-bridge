package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	mk := func(origin, host string) *http.Request {
		r := httptest.NewRequest("GET", "http://"+host+"/ws", nil)
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	if !sameOrigin(mk("", "hub.local")) || !sameOrigin(mk("https://hub.local", "hub.local")) {
		t.Fatal("same origin / no origin must pass")
	}
	if sameOrigin(mk("https://evil.example", "hub.local")) {
		t.Fatal("foreign origin must be refused")
	}
}

func TestPathAllowed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENT_HUB_WORKSPACE_ROOTS", "")
	ok := filepath.Join(home, "proj", "a.go")
	_ = os.MkdirAll(filepath.Dir(ok), 0o755)
	_ = os.WriteFile(ok, []byte("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(home, ".ssh"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("k"), 0o600)
	_ = os.MkdirAll(filepath.Join(home, ".agent-hub", "uploads"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".agent-hub", "uploads", "i.png"), []byte("p"), 0o644)
	_ = os.WriteFile(filepath.Join(home, ".agent-hub", "agent-hub.db"), []byte("d"), 0o644)
	_ = os.WriteFile(filepath.Join(home, "proj", ".env"), []byte("s"), 0o644)
	_ = os.Symlink(filepath.Join(home, ".ssh", "id_ed25519"), filepath.Join(home, "proj", "link"))
	cases := map[string]bool{
		ok: true,
		filepath.Join(home, ".ssh", "id_ed25519"):             false,
		filepath.Join(home, ".agent-hub", "uploads", "i.png"): true,
		filepath.Join(home, ".agent-hub", "agent-hub.db"):     false,
		filepath.Join(home, "proj", ".env"):                   false,
		filepath.Join(home, "proj", "link"):                   false, // symlink into ~/.ssh
		"/etc/passwd":                                         false,
		filepath.Join(home, "proj", "..", "..", "etc"):        false,
		"relative/path":                                       false,
	}
	for p, want := range cases {
		if got := PathAllowed(p); got != want {
			t.Errorf("PathAllowed(%q)=%v want %v", p, got, want)
		}
	}
}

func TestAuthMiddleware(t *testing.T) {
	t.Setenv("AGENT_HUB_TOKEN", "s3cret-token")
	h := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	do := func(mod func(*http.Request)) int {
		r := httptest.NewRequest("GET", "/api/x", nil)
		if mod != nil {
			mod(r)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if do(nil) != 401 {
		t.Fatal("no credentials must be 401")
	}
	if do(func(r *http.Request) { r.Header.Set("Authorization", "Bearer s3cret-token") }) != 200 {
		t.Fatal("bearer must pass")
	}
	if do(func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "hub_token", Value: "s3cret-token"}) }) != 200 {
		t.Fatal("cookie must pass")
	}
	if do(func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") }) != 401 {
		t.Fatal("wrong token must be 401")
	}
}

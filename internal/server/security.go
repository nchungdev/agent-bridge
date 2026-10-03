package server

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// sameOrigin lets browsers connect only from the page this hub serves, so a
// foreign website cannot open WebSockets to a locally reachable hub (CSWSH).
// Non-browser clients send no Origin header and are allowed.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && u.Host == r.Host
}

// workspaceRoots are the directories the GUI may browse/read. Override with
// AGENT_HUB_WORKSPACE_ROOTS (colon separated); default is the user's home.
func workspaceRoots() []string {
	if v := os.Getenv("AGENT_HUB_WORKSPACE_ROOTS"); v != "" {
		var out []string
		for _, p := range strings.Split(v, ":") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, filepath.Clean(p))
			}
		}
		return out
	}
	if h, err := os.UserHomeDir(); err == nil {
		return []string{h}
	}
	return nil
}

// credential locations that must never be served, even inside an allowed root.
var deniedHomeDirs = []string{".ssh", ".gnupg", ".aws", ".codex", ".claude", ".gemini", ".config/gcloud", ".config/gh", ".agent-hub", ".docker", ".kube", ".netrc"}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// PathAllowed reports whether p (after symlink resolution) lies in an allowed
// root and outside credential directories. The uploads dir is the one
// exception under ~/.agent-hub.
func PathAllowed(p string) bool {
	if p == "" || !filepath.IsAbs(p) {
		return false
	}
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		if h, err := filepath.EvalSymlinks(home); err == nil {
			home = h
		}
		if within(filepath.Join(home, ".agent-hub", "uploads"), p) {
			return true
		}
		for _, d := range deniedHomeDirs {
			if within(filepath.Join(home, d), p) {
				return false
			}
		}
	}
	for _, root := range workspaceRoots() {
		if r, err := filepath.EvalSymlinks(root); err == nil {
			root = r
		}
		if within(root, p) {
			base := filepath.Base(p)
			if base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") {
				return false
			}
			return true
		}
	}
	return false
}

func forbidPath(w http.ResponseWriter) {
	httpError(w, errForbidden, http.StatusForbidden)
}

type constErr string

func (e constErr) Error() string { return string(e) }

const errForbidden = constErr("path is outside the allowed workspace roots")

// authMiddleware enforces AGENT_HUB_TOKEN when set: a Bearer header or a
// hub_token cookie (set by visiting /?token=...). With no token configured the
// hub relies on its loopback bind and the fronting gateway, as before.
func authMiddleware(next http.Handler) http.Handler {
	token := os.Getenv("AGENT_HUB_TOKEN")
	if token == "" {
		return next
	}
	eq := func(a string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(token)) == 1 }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("token"); t != "" && eq(t) {
			http.SetCookie(w, &http.Cookie{Name: "hub_token", Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"})
			http.Redirect(w, r, r.URL.Path, http.StatusFound)
			return
		}
		if h := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); h != "" && eq(h) {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie("hub_token"); err == nil && eq(c.Value) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// terminalEnabled: the unauthenticated shell endpoints can be switched off with AGENT_HUB_TERMINAL=0.
func terminalEnabled() bool { return os.Getenv("AGENT_HUB_TERMINAL") != "0" }

package server

import (
	"net/http/httptest"
	"testing"
)

func TestInternalRemote(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:5000":  true,
		"172.17.0.2:5000": true,
		"192.168.1.5:80":  true,
		"8.8.8.8:80":      false,
		"[::1]:80":        true,
		"bogus":           false,
	}
	for addr, want := range cases {
		r := httptest.NewRequest("POST", "/api/admin/restart", nil)
		r.RemoteAddr = addr
		if got := internalRemote(r); got != want {
			t.Errorf("internalRemote(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestAdminRestartRejectsPublicAndCrossOrigin(t *testing.T) {
	for _, tc := range []struct{ addr, origin string }{{"8.8.8.8:1", ""}, {"127.0.0.1:1", "http://evil.example"}} {
		r := httptest.NewRequest("POST", "/api/admin/restart", nil)
		r.RemoteAddr, r.Host = tc.addr, "hub.local"
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		handleAdminRestart(w, r)
		if w.Code != 403 {
			t.Errorf("%+v: got %d, want 403", tc, w.Code)
		}
	}
}

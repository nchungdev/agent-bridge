package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestAdminUpdateSecurity(t *testing.T) {
	for _, tc := range []struct{ addr, origin string }{
		{"8.8.8.8:1", ""},
		{"127.0.0.1:1", "http://evil.example"},
	} {
		r := httptest.NewRequest("GET", "/api/admin/update/status", nil)
		r.RemoteAddr, r.Host = tc.addr, "hub.local"
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		handleGetUpdateStatus(w, r)
		if w.Code != 403 {
			t.Errorf("status %+v: got %d, want 403", tc, w.Code)
		}

		w2 := httptest.NewRecorder()
		r2 := httptest.NewRequest("POST", "/api/admin/update/apply", nil)
		r2.RemoteAddr, r2.Host = tc.addr, "hub.local"
		if tc.origin != "" {
			r2.Header.Set("Origin", tc.origin)
		}
		handleApplyUpdate(w2, r2)
		if w2.Code != 403 {
			t.Errorf("apply %+v: got %d, want 403", tc, w2.Code)
		}
	}
}

func TestGetUpdateStatus(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/admin/update/status", nil)
	r.RemoteAddr = "127.0.0.1:8088"
	r.Host = "127.0.0.1:8088"
	w := httptest.NewRecorder()

	handleGetUpdateStatus(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status UpdateStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to parse json: %v", err)
	}

	if status.CurrentCommit == "" {
		t.Errorf("expected non-empty CurrentCommit")
	}
	if status.Branch == "" {
		t.Errorf("expected non-empty Branch")
	}
}

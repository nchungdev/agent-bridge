package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os/exec"
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

func TestFormatVersion(t *testing.T) {
	for _, c := range []struct {
		tag   string
		build int
		want  string
	}{
		{"v1.0.1", 1, "v1.0.1 (build 01)"},
		{"v1.0.1", 2, "v1.0.1 (build 02)"},
		{"v1.0.1", 99, "v1.0.1 (build 99)"},
		{"v1.0.1", 100, "v1.0.1 (build 100)"}, // past the limit: shown as is (BuildOverflow flags it)
		{"", 5, "v1.0.0 (build 05)"},
		{"v1.2.3", 0, "v1.2.3"},
	} {
		if got := formatVersion(c.tag, c.build); got != c.want {
			t.Errorf("formatVersion(%q, %d) = %q, want %q", c.tag, c.build, got, c.want)
		}
	}
}

func TestParseRelease(t *testing.T) {
	for _, c := range []struct {
		tag     string
		version string
		build   int
		ok      bool
	}{
		{"v1.0.1", "v1.0.1", 1, true},
		{"v1.0.1-b02", "v1.0.1", 2, true},
		{"v1.0.1-b99", "v1.0.1", 99, true},
		{"v10.20.30", "v10.20.30", 1, true},
		{"v1.0.1-b01", "", 0, false},  // build 01 is the plain tag
		{"v1.0.1-b00", "", 0, false},  // there is no build 0
		{"v1.0.1-b100", "", 0, false}, // past MaxBuild: the next patch version is required
		{"v1.0.1-rc1", "", 0, false},
		{"1.0.1", "", 0, false},
		{"v1.0", "", 0, false},
		{"", "", 0, false},
	} {
		v, b, ok := parseRelease(c.tag)
		if v != c.version || b != c.build || ok != c.ok {
			t.Errorf("parseRelease(%q) = (%q, %d, %v), want (%q, %d, %v)", c.tag, v, b, ok, c.version, c.build, c.ok)
		}
	}
}

func TestReleaseOrder(t *testing.T) {
	order := [][2]any{ // ascending
		{"v0.1.0", 1}, {"v1.0.0", 1}, {"v1.0.1", 1}, {"v1.0.1", 2}, {"v1.0.1", 99}, {"v1.0.2", 1}, {"v1.0.10", 1}, {"v1.1.0", 1}, {"v2.0.0", 1},
	}
	for i := 1; i < len(order); i++ {
		a := releaseKey(order[i-1][0].(string), order[i-1][1].(int))
		b := releaseKey(order[i][0].(string), order[i][1].(int))
		if !releaseLess(a, b) || releaseLess(b, a) {
			t.Errorf("%v must sort below %v", order[i-1], order[i])
		}
	}
}

// versionAt reads the build from release tags: it never counts commits, and the highest release wins.
func TestVersionAt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-c", "user.email=t@example.test", "-c", "user.name=t", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)
		cmd := exec.Command("git", full...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	commit := func(msg string) { git("commit", "--allow-empty", "-q", "-m", msg) }
	ctx := context.Background()
	check := func(wantVersion string, wantBuild int, why string) {
		t.Helper()
		v, b := versionAt(ctx, dir, "HEAD")
		if v != wantVersion || b != wantBuild {
			t.Errorf("%s: versionAt = (%q, %d), want (%q, %d)", why, v, b, wantVersion, wantBuild)
		}
	}

	git("init", "-q")
	commit("a")
	check("", 0, "no release tag yet")

	git("tag", "v1.0.0")
	check("v1.0.0", 1, "the plain tag is build 01")
	commit("b")
	commit("c")
	check("v1.0.0", 1, "commits do not raise the build")

	git("tag", "v0.1.0") // a lower version tagged later must not win just because it is nearer
	check("v1.0.0", 1, "the highest version wins, not the nearest tag")

	git("tag", "v1.0.1")
	check("v1.0.1", 1, "a new patch tag is build 01")
	commit("hotfix")
	git("tag", "v1.0.1-b02")
	check("v1.0.1", 2, "a hotfix build is tagged on purpose: build 02")
	commit("hotfix 2")
	check("v1.0.1", 2, "an untagged commit after it is still build 02")
	git("tag", "v1.0.1-b10")
	git("tag", "v1.0.1-b09")
	check("v1.0.1", 10, "builds compare as numbers: 10 is above 09")

	git("tag", "v1.0.1-rc1") // not a release tag
	git("tag", "v1.0.2-b100")
	check("v1.0.1", 10, "malformed tags are ignored")

	git("tag", "v1.0.2")
	check("v1.0.2", 1, "the next patch restarts at 01")
	git("tag", "v1.0.10")
	check("v1.0.10", 1, "versions compare numerically: 1.0.10 is above 1.0.2")
}

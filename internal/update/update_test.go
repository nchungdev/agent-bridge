package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tarball builds a release archive the way the release workflow does: a directory holding the binary and a README.
func tarball(t *testing.T, tag, plat, binary string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	dir := fmt.Sprintf("agent-bridge_%s_%s", tag, plat)
	for _, f := range []struct{ name, body string }{{dir + "/README.md", "readme\n"}, {dir + "/agent-bridge", binary}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type fakeRelease struct {
	tag       string
	draft     bool
	platforms []string // "linux_amd64", ...
	binary    string
	badSum    bool // publish a checksum that does not match the archive
}

// serve starts a fake GitHub that publishes rels, and returns an Updater pointed at it as linux/amd64.
func serve(t *testing.T, rels []fakeRelease) *Updater {
	t.Helper()
	files := map[string][]byte{}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	var out []Release
	for _, r := range rels {
		rel := Release{Tag: r.tag, Draft: r.draft}
		for _, plat := range r.platforms {
			name := fmt.Sprintf("agent-bridge_%s_%s.tar.gz", r.tag, plat)
			data := tarball(t, r.tag, plat, r.binary)
			sum := sha256.Sum256(data)
			hexsum := hex.EncodeToString(sum[:])
			if r.badSum {
				hexsum = strings.Repeat("0", 64)
			}
			files["/dl/"+name] = data
			files["/dl/"+name+".sha256"] = []byte(hexsum + "  " + name + "\n")
			rel.Assets = append(rel.Assets,
				Asset{Name: name, URL: srv.URL + "/dl/" + name, Size: int64(len(data))},
				Asset{Name: name + ".sha256", URL: srv.URL + "/dl/" + name + ".sha256"})
		}
		out = append(out, rel)
	}
	mux.HandleFunc("/repos/o/r/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	})
	return &Updater{API: srv.URL, Repo: "o/r", Client: srv.Client(), GOOS: "linux", GOARCH: "amd64"}
}

func writeExe(t *testing.T, content string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "agent-bridge")
	if err := os.WriteFile(exe, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLatestPicksTheNewestReleaseForThisPlatform(t *testing.T) {
	u := serve(t, []fakeRelease{
		{tag: "v1.0.0", platforms: []string{"linux_amd64"}},
		{tag: "v1.0.1-b02", platforms: []string{"linux_amd64"}},
		{tag: "v1.0.1", platforms: []string{"linux_amd64"}},
		{tag: "v1.0.3", draft: true, platforms: []string{"linux_amd64"}},    // not published
		{tag: "nightly", platforms: []string{"linux_amd64"}},                // not a release tag
		{tag: "v1.0.2", platforms: []string{"darwin_arm64"}},                // no build for linux/amd64
		{tag: "v1.0.1-b100", platforms: []string{"linux_amd64"}},            // past build 99: not valid
	})
	r, err := u.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || r.Tag != "v1.0.1-b02" {
		t.Fatalf("Latest = %+v, want v1.0.1-b02 (the highest version and build with a linux/amd64 build)", r)
	}

	none := serve(t, []fakeRelease{{tag: "v1.0.0", draft: true, platforms: []string{"linux_amd64"}}})
	if r, err := none.Latest(context.Background()); err != nil || r != nil {
		t.Fatalf("no published release: got (%+v, %v), want (nil, nil)", r, err)
	}
}

func TestApplyReplacesTheBinaryAndKeepsThePrevious(t *testing.T) {
	u := serve(t, []fakeRelease{{tag: "v1.0.1", platforms: []string{"linux_amd64"}, binary: "NEW BINARY"}})
	r, err := u.Latest(context.Background())
	if err != nil || r == nil {
		t.Fatalf("Latest = (%+v, %v)", r, err)
	}
	exe := writeExe(t, "OLD BINARY")
	if err := u.Apply(context.Background(), r, exe); err != nil {
		t.Fatal(err)
	}
	if got := read(t, exe); got != "NEW BINARY" {
		t.Errorf("binary = %q, want the new one", got)
	}
	if got := read(t, exe+".prev"); got != "OLD BINARY" {
		t.Errorf("previous = %q, want the old one", got)
	}
	if st, err := os.Stat(exe); err != nil || st.Mode()&0o111 == 0 {
		t.Errorf("the new binary must be executable: %v %v", st, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 2 {
		t.Errorf("expected only the binary and its .prev, got %d entries", len(entries))
	}
}

func TestApplyRefusesADownloadThatDoesNotMatchItsChecksum(t *testing.T) {
	u := serve(t, []fakeRelease{{tag: "v1.0.1", platforms: []string{"linux_amd64"}, binary: "TAMPERED", badSum: true}})
	r, _ := u.Latest(context.Background())
	exe := writeExe(t, "OLD BINARY")
	err := u.Apply(context.Background(), r, exe)
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("Apply error = %v, want a checksum error", err)
	}
	if got := read(t, exe); got != "OLD BINARY" {
		t.Errorf("the binary changed to %q despite the bad checksum", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Errorf("a failed update must leave nothing behind, got %d entries", len(entries))
	}
}

func TestApplyNeedsAVerifiedBuildForThisPlatform(t *testing.T) {
	u := serve(t, nil)
	r := &Release{Tag: "v1.0.1", Assets: []Asset{{Name: "agent-bridge_v1.0.1_linux_amd64.tar.gz", URL: "http://invalid.example/x"}}} // no .sha256
	err := u.Apply(context.Background(), r, writeExe(t, "OLD"))
	if err == nil || !strings.Contains(err.Error(), "no verified build") {
		t.Fatalf("Apply error = %v, want 'no verified build'", err)
	}
}

func TestRollback(t *testing.T) {
	u := serve(t, []fakeRelease{{tag: "v1.0.1", platforms: []string{"linux_amd64"}, binary: "NEW BINARY"}})
	r, _ := u.Latest(context.Background())
	exe := writeExe(t, "OLD BINARY")
	if err := u.Apply(context.Background(), r, exe); err != nil {
		t.Fatal(err)
	}
	if err := Rollback(exe); err != nil {
		t.Fatal(err)
	}
	if got := read(t, exe); got != "OLD BINARY" {
		t.Errorf("after rollback the binary is %q, want the old one", got)
	}
	if err := Rollback(exe); err == nil {
		t.Error("a second rollback has nothing to go back to and must fail")
	}
}

// Package update replaces the running binary with a newer release from GitHub Releases: it picks the newest
// release that has a build for this platform, verifies the download against its published sha256, and swaps it
// in, keeping the previous binary next to it so the change can be undone. It does not restart anything.
package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nchungdev/agent-bridge/internal/release"
)

const (
	// DefaultRepo is where releases are published.
	DefaultRepo = "nchungdev/agent-bridge"
	// DefaultAPI is the GitHub API base URL.
	DefaultAPI = "https://api.github.com"

	binaryName   = "agent-bridge"
	maxBinary    = 256 << 20 // refuse to unpack more than this
	maxSumLength = 4096
)

// Asset is one downloadable file of a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Release is a published GitHub release.
type Release struct {
	Tag    string  `json:"tag_name"`
	Draft  bool    `json:"draft"`
	Assets []Asset `json:"assets"`
}

// Updater talks to GitHub. The zero value is not usable: call New.
type Updater struct {
	API    string // GitHub API base URL
	Repo   string // "owner/name"
	Client *http.Client
	// GOOS and GOARCH pick the build to download (this machine's by default).
	GOOS, GOARCH string
}

// New returns an Updater for the project's releases on this platform.
func New() *Updater {
	return &Updater{API: DefaultAPI, Repo: DefaultRepo, Client: &http.Client{Timeout: 10 * time.Minute}, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
}

// AssetName is the file name of a release's build for a platform.
func AssetName(tag, goos, goarch string) string {
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", binaryName, tag, goos, goarch)
}

func find(assets []Asset, name string) *Asset {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

func (u *Updater) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "agent-bridge-updater")
	req.Header.Set("Accept", "application/vnd.github+json, application/octet-stream")
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

// Latest returns the newest release (by version, then build; not by publication date) that is published and
// has a build for this platform. It returns nil, with no error, when there is none.
func (u *Updater) Latest(ctx context.Context) (*Release, error) {
	resp, err := u.get(ctx, fmt.Sprintf("%s/repos/%s/releases?per_page=50", strings.TrimRight(u.API, "/"), u.Repo))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var all []Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&all); err != nil {
		return nil, fmt.Errorf("read releases: %w", err)
	}
	var best *Release
	for i := range all {
		r := &all[i]
		if r.Draft || find(r.Assets, AssetName(r.Tag, u.GOOS, u.GOARCH)) == nil {
			continue
		}
		if _, _, ok := release.Parse(r.Tag); !ok {
			continue
		}
		if best == nil || release.Newer(r.Tag, best.Tag) {
			best = r
		}
	}
	return best, nil
}

// fetchSum reads a "<hex sha256>  <file name>" checksum file.
func (u *Updater) fetchSum(ctx context.Context, url string) (string, error) {
	resp, err := u.get(ctx, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxSumLength))
	if err != nil {
		return "", err
	}
	f := strings.Fields(string(b))
	if len(f) == 0 || len(f[0]) != sha256.Size*2 {
		return "", errors.New("checksum file is not a sha256")
	}
	return strings.ToLower(f[0]), nil
}

// download saves url to a temporary file and returns its path and sha256.
func (u *Updater) download(ctx context.Context, url string) (file, sum string, err error) {
	resp, err := u.get(ctx, url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	f, err := os.CreateTemp("", "agent-bridge-download-*")
	if err != nil {
		return "", "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", "", err
	}
	return f.Name(), hex.EncodeToString(h.Sum(nil)), nil
}

// unpack writes the binary found in a release tarball to dst.
func unpack(tarball string, dst *os.File) error {
	f, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return errors.New("the release contains no " + binaryName + " binary")
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || path.Base(hdr.Name) != binaryName {
			continue
		}
		n, err := io.Copy(dst, io.LimitReader(tr, maxBinary+1))
		if err != nil {
			return err
		}
		if n > maxBinary {
			return errors.New("the binary in the release is implausibly large")
		}
		return nil
	}
}

// Apply downloads release r for this platform and swaps it in for the executable at exe. Nothing changes unless
// the download matches its published sha256. The previous executable is kept as exe+".prev".
func (u *Updater) Apply(ctx context.Context, r *Release, exe string) error {
	name := AssetName(r.Tag, u.GOOS, u.GOARCH)
	tarAsset, sumAsset := find(r.Assets, name), find(r.Assets, name+".sha256")
	if tarAsset == nil || sumAsset == nil {
		return fmt.Errorf("release %s has no verified build for %s/%s", r.Tag, u.GOOS, u.GOARCH)
	}
	want, err := u.fetchSum(ctx, sumAsset.URL)
	if err != nil {
		return fmt.Errorf("checksum: %w", err)
	}
	tarball, got, err := u.download(ctx, tarAsset.URL)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer os.Remove(tarball)
	if got != want {
		return fmt.Errorf("download is corrupt or tampered with: sha256 %s, expected %s", got, want)
	}

	// the new binary is staged next to the old one so the final rename stays on one filesystem
	staged, err := os.CreateTemp(filepath.Dir(exe), ".agent-bridge-update-*")
	if err != nil {
		return fmt.Errorf("stage the new binary: %w", err)
	}
	stagedName := staged.Name()
	defer os.Remove(stagedName) // gone already after a successful rename
	if err := unpack(tarball, staged); err != nil {
		_ = staged.Close()
		return fmt.Errorf("unpack: %w", err)
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if err := os.Chmod(stagedName, 0o755); err != nil {
		return err
	}

	prev := exe + ".prev"
	_ = os.Remove(prev)
	if err := os.Rename(exe, prev); err != nil {
		return fmt.Errorf("keep the previous version: %w", err)
	}
	if err := os.Rename(stagedName, exe); err != nil {
		_ = os.Rename(prev, exe) // put the old one back: never leave the app without a binary
		return fmt.Errorf("install the new version: %w", err)
	}
	return nil
}

// Rollback puts the previous binary (kept by Apply) back in place of exe.
func Rollback(exe string) error {
	prev := exe + ".prev"
	if _, err := os.Stat(prev); err != nil {
		return errors.New("there is no previous version to go back to")
	}
	failed := exe + ".failed"
	_ = os.Remove(failed)
	if err := os.Rename(exe, failed); err != nil {
		return err
	}
	if err := os.Rename(prev, exe); err != nil {
		_ = os.Rename(failed, exe)
		return err
	}
	_ = os.Remove(failed)
	return nil
}

// Executable is the path of the running binary with symlinks resolved, so an update replaces the real file.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

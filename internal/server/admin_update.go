package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nchungdev/agent-bridge/internal/update"
)

type UpdateStatus struct {
	CurrentVersion string   `json:"current_version"`
	LatestVersion  string   `json:"latest_version"`
	CurrentBuild   int      `json:"current_build,omitempty"`
	LatestBuild    int      `json:"latest_build,omitempty"`
	CurrentCommit  string   `json:"current_commit"`
	CurrentMessage string   `json:"current_message"`
	CurrentDate    string   `json:"current_date"`
	Branch         string   `json:"branch"`
	RemoteCommit   string   `json:"remote_commit"`
	HasUpdate      bool     `json:"has_update"`
	CommitsBehind  int      `json:"commits_behind"`
	Commits        []string `json:"commits"`
	IsUpdating     bool     `json:"is_updating"`
	Step           string   `json:"step"` // "idle", "checking", "pulling", "building_web", "building_binary", "restarting", "success", "error"
	Error          string   `json:"error,omitempty"`
	Logs           []string `json:"logs"`
	LastChecked    string   `json:"last_checked,omitempty"`
}

type UpdateManager struct {
	mu     sync.Mutex
	status UpdateStatus
	// release mode: the newest GitHub release seen and when it was last asked for
	latest  *update.Release
	fetched time.Time
}

var globalUpdater = &UpdateManager{
	status: UpdateStatus{
		CurrentVersion: "v1.0.0",
		LatestVersion:  "v1.0.0",
		Step:           "idle",
		Logs:           []string{},
	},
}

func init() {
	// Auto check on startup and periodically every 24 hours
	go func() {
		time.Sleep(3 * time.Second)
		_, _ = globalUpdater.CheckUpdate(context.Background(), true)

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			_, _ = globalUpdater.CheckUpdate(context.Background(), true)
		}
	}()
}

func getRepoDir() string {
	// 1. Explicit override via environment variable
	if custom := os.Getenv("AGENT_BRIDGE_REPO_DIR"); custom != "" {
		if _, err := os.Stat(filepath.Join(custom, ".git")); err == nil {
			return custom
		}
	}
	// 2. Working directory or ancestors
	wd, err := os.Getwd()
	if err == nil {
		for cur := wd; cur != "" && cur != "/" && cur != "."; cur = filepath.Dir(cur) {
			if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
				return cur
			}
			if cur == filepath.Dir(cur) {
				break
			}
		}
	}
	// 3. Executable directory or ancestors
	exe, err := os.Executable()
	if err == nil {
		for cur := filepath.Dir(exe); cur != "" && cur != "/" && cur != "."; cur = filepath.Dir(cur) {
			if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
				return cur
			}
			if cur == filepath.Dir(cur) {
				break
			}
		}
	}
	// 4. Common repo locations in user home
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates := []string{
			filepath.Join(home, "AI Workspace", "agent-bridge"),
			filepath.Join(home, "agent-bridge"),
			filepath.Join(home, "Projects", "agent-bridge"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(filepath.Join(c, ".git")); err == nil {
				return c
			}
		}
	}
	return wd
}

func runGitCmd(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// A build is a release tag someone pushed on purpose, never a side effect of committing:
//
//	v1.0.1       = version 1.0.1, build 01
//	v1.0.1-b02   = version 1.0.1, build 02 (a hotfix build), ... up to -b99
//
// After build 99 the next patch version (v1.0.2) must be tagged, which restarts the numbering at 01.
const MaxBuild = 99

var releaseTagRe = regexp.MustCompile(`^(v\d+\.\d+\.\d+)(?:-b(\d{2}))?$`)

// parseRelease splits a release tag into its version ("v1.0.1") and build number (1 for the plain tag).
// Anything else (v1.0.3-rc1, -b00, -b01, a build over MaxBuild) is not a release tag.
func parseRelease(tag string) (version string, build int, ok bool) {
	m := releaseTagRe.FindStringSubmatch(strings.TrimSpace(tag))
	if m == nil {
		return "", 0, false
	}
	if m[2] == "" {
		return m[1], 1, true
	}
	b, _ := strconv.Atoi(m[2])
	if b < 2 || b > MaxBuild { // -b01 would be the plain tag, and there is no build 0
		return "", 0, false
	}
	return m[1], b, true
}

// releaseKey orders release tags: version numerically, then build.
func releaseKey(version string, build int) [4]int {
	var k [4]int
	for i, p := range strings.Split(strings.TrimPrefix(version, "v"), ".") {
		if i < 3 {
			k[i], _ = strconv.Atoi(p)
		}
	}
	k[3] = build
	return k
}

func releaseLess(a, b [4]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// versionAt returns the newest release (highest version, then build; not merely the nearest tag) reachable from
// ref. With no release tag it returns ("", 0).
func versionAt(ctx context.Context, dir, ref string) (version string, build int) {
	out, err := runGitCmd(ctx, dir, "tag", "--merged", ref, "--list", "v[0-9]*")
	if err != nil {
		return "", 0
	}
	var best [4]int
	for _, t := range strings.Split(out, "\n") {
		v, b, ok := parseRelease(t)
		if !ok {
			continue
		}
		if k := releaseKey(v, b); version == "" || releaseLess(best, k) {
			version, build, best = v, b, k
		}
	}
	return version, build
}

func formatVersion(tag string, build int) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = "v1.0.0"
	}
	if build > 0 {
		return fmt.Sprintf("%s (build %02d)", tag, build)
	}
	return tag
}

// CheckUpdate queries git for current and remote version/commit info.
func (u *UpdateManager) CheckUpdate(ctx context.Context, fetchRemote bool) (*UpdateStatus, error) {
	if updateMode() != "git" { // release and off do not look at a checkout
		return u.checkRelease(ctx, fetchRemote)
	}
	u.mu.Lock()
	defer u.mu.Unlock()

	dir := getRepoDir()

	// 1. Current commit & branch
	branch, err := runGitCmd(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		branch = "main"
	}
	currCommit, err := runGitCmd(ctx, dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	currMsg, _ := runGitCmd(ctx, dir, "log", "-1", "--format=%s")
	currDate, _ := runGitCmd(ctx, dir, "log", "-1", "--format=%cd", "--date=relative")

	rawCurrTag, currBuild := versionAt(ctx, dir, "HEAD")
	if rawCurrTag == "" {
		rawCurrTag = "v1.0.0"
	}
	currVersion := formatVersion(rawCurrTag, currBuild)

	u.status.Branch = branch
	u.status.CurrentCommit = currCommit
	u.status.CurrentMessage = currMsg
	u.status.CurrentDate = currDate
	u.status.CurrentVersion = currVersion
	u.status.CurrentBuild = currBuild

	if fetchRemote {
		// Fetch latest info and tags from origin with 20s timeout
		fCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		_, _ = runGitCmd(fCtx, dir, "fetch", "--tags", "origin", branch)
		cancel()
	}

	remoteCommit, _ := runGitCmd(ctx, dir, "rev-parse", "--short", "origin/"+branch)
	u.status.RemoteCommit = remoteCommit

	// Latest release tag and build on the remote branch
	rawLatestTag, remoteBuild := versionAt(ctx, dir, "origin/"+branch)
	if rawLatestTag == "" {
		rawLatestTag = rawCurrTag
	}
	if remoteBuild == 0 {
		remoteBuild = currBuild
	}

	latestVersion := formatVersion(rawLatestTag, remoteBuild)

	var commits []string
	behindCount := 0
	if remoteCommit != "" && remoteCommit != currCommit {
		logOut, err := runGitCmd(ctx, dir, "log", "HEAD..origin/"+branch, "--oneline")
		if err == nil && logOut != "" {
			for _, line := range strings.Split(logOut, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					commits = append(commits, line)
				}
			}
		}
		countStr, err := runGitCmd(ctx, dir, "rev-list", "--count", "HEAD..origin/"+branch)
		if err == nil {
			behindCount, _ = strconv.Atoi(strings.TrimSpace(countStr))
		}
	}

	// An update is a newer release (a tag someone pushed on purpose), not merely newer commits: pushing code
	// does not make a build, so it does not announce an update either.
	hasUpdate := releaseLess(releaseKey(rawCurrTag, currBuild), releaseKey(rawLatestTag, remoteBuild))
	if !hasUpdate {
		latestVersion = currVersion
		remoteBuild = currBuild
	}

	u.status.LatestVersion = latestVersion
	u.status.LatestBuild = remoteBuild
	u.status.HasUpdate = hasUpdate
	u.status.CommitsBehind = behindCount
	u.status.Commits = commits
	u.status.LastChecked = time.Now().Format(time.RFC3339)

	copyStatus := u.status
	return &copyStatus, nil
}

// HTTP Handlers

func handleGetUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if !internalRemote(r) || !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	status, err := globalUpdater.CheckUpdate(r.Context(), false)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	jsonResponse(w, status)
}

func handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	if !internalRemote(r) || !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	status, err := globalUpdater.CheckUpdate(r.Context(), true)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	jsonResponse(w, status)
}

func handleApplyUpdate(w http.ResponseWriter, r *http.Request) {
	if !internalRemote(r) || !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := globalUpdater.ApplyUpdate(); err != nil {
		httpError(w, err, http.StatusConflict)
		return
	}
	jsonResponse(w, map[string]any{"success": true, "message": "Quá trình cập nhật đã bắt đầu"})
}

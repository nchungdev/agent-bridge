package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
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

func formatVersion(tag string, build int) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = "v1.0.0"
	}
	if build > 0 {
		return fmt.Sprintf("%s (build %d)", tag, build)
	}
	return tag
}

// CheckUpdate queries git for current and remote version/commit info.
func (u *UpdateManager) CheckUpdate(ctx context.Context, fetchRemote bool) (*UpdateStatus, error) {
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

	rawCurrTag, _ := runGitCmd(ctx, dir, "describe", "--tags", "--abbrev=0")
	if rawCurrTag == "" {
		rawCurrTag = "v1.0.0"
	}
	currBuildStr, _ := runGitCmd(ctx, dir, "rev-list", "--count", "HEAD")
	currBuild, _ := strconv.Atoi(strings.TrimSpace(currBuildStr))
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

	// Latest tag on remote
	rawLatestTag, _ := runGitCmd(ctx, dir, "describe", "--tags", "--abbrev=0", "origin/"+branch)
	if rawLatestTag == "" {
		tagsOut, _ := runGitCmd(ctx, dir, "tag", "--sort=-v:refname")
		if tagsOut != "" {
			parts := strings.Split(tagsOut, "\n")
			if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
				rawLatestTag = strings.TrimSpace(parts[0])
			}
		}
	}
	if rawLatestTag == "" {
		rawLatestTag = rawCurrTag
	}

	remoteBuildStr, _ := runGitCmd(ctx, dir, "rev-list", "--count", "origin/"+branch)
	remoteBuild, _ := strconv.Atoi(strings.TrimSpace(remoteBuildStr))
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

	// If local is ahead of remote and no new remote commits, keep latest matching current
	if remoteBuild < currBuild && behindCount == 0 && rawLatestTag == rawCurrTag {
		latestVersion = currVersion
		remoteBuild = currBuild
	}

	u.status.LatestVersion = latestVersion
	u.status.LatestBuild = remoteBuild
	u.status.HasUpdate = (remoteBuild > currBuild) || (rawLatestTag != rawCurrTag) || behindCount > 0
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

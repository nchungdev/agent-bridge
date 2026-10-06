package server

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	// 1. Working directory
	wd, err := os.Getwd()
	if err == nil {
		if _, err := os.Stat(filepath.Join(wd, ".git")); err == nil {
			return wd
		}
	}
	// 2. Executable directory
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
	}
	// 3. Fallback standard workspace
	const defaultDir = "/home/chungnh/AI Workspace/agent-bridge"
	if _, err := os.Stat(filepath.Join(defaultDir, ".git")); err == nil {
		return defaultDir
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

// ApplyUpdate executes git pull, web build, Go compile, and service restart.
func (u *UpdateManager) ApplyUpdate() error {
	u.mu.Lock()
	if u.status.IsUpdating {
		u.mu.Unlock()
		return fmt.Errorf("tiến trình cập nhật đang chạy")
	}
	u.status.IsUpdating = true
	u.status.Step = "pulling"
	u.status.Error = ""
	u.status.Logs = []string{fmt.Sprintf("[%s] Bắt đầu quá trình cập nhật...", time.Now().Format("15:04:05"))}
	u.mu.Unlock()

	go u.runPipeline()
	return nil
}

func (u *UpdateManager) logStep(msg string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	u.status.Logs = append(u.status.Logs, entry)
	log.Printf("[update] %s", msg)
}

func (u *UpdateManager) runPipeline() {
	dir := getRepoDir()
	envPath := "/usr/local/go/bin:/usr/bin:/bin:" + os.Getenv("PATH")
	homeDir, _ := os.UserHomeDir()
	tmpDir := filepath.Join(homeDir, ".tmp")
	_ = os.MkdirAll(tmpDir, 0755)

	fail := func(step string, err error) {
		u.mu.Lock()
		u.status.IsUpdating = false
		u.status.Step = "error"
		u.status.Error = fmt.Sprintf("%s: %v", step, err)
		u.status.Logs = append(u.status.Logs, fmt.Sprintf("[%s] ❌ Lỗi tại bước %s: %v", time.Now().Format("15:04:05"), step, err))
		u.mu.Unlock()
		log.Printf("[update] error in %s: %v", step, err)
	}

	// Step 1: Git Pull
	u.logStep("Đang tải mã nguồn mới nhất từ Git (git pull origin)...")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	pullOut, err := runGitCmd(ctx, dir, "pull", "--rebase", "origin", u.status.Branch)
	cancel()
	if err != nil {
		fail("Git Pull", err)
		return
	}
	u.logStep(fmt.Sprintf("Git Pull hoàn tất: %s", pullOut))

	// Step 2: Build Web frontend
	u.mu.Lock()
	u.status.Step = "building_web"
	u.mu.Unlock()
	u.logStep("Đang biên dịch giao diện Frontend (npm run build)...")

	npmCmd := exec.Command("npm", "run", "build")
	npmCmd.Dir = filepath.Join(dir, "web")
	npmCmd.Env = append(os.Environ(), "PATH="+envPath, "TMPDIR="+tmpDir)
	var npmBuf bytes.Buffer
	npmCmd.Stdout = &npmBuf
	npmCmd.Stderr = &npmBuf
	if err := npmCmd.Run(); err != nil {
		fail("Build Web", fmt.Errorf("%s: %s", err, npmBuf.String()))
		return
	}
	u.logStep("Frontend build thành công (Vite bundle ready).")

	// Step 3: Build Go binary
	u.mu.Lock()
	u.status.Step = "building_binary"
	u.mu.Unlock()
	u.logStep("Đang biên dịch nhị phân Go (go build agent-bridge)...")

	goBin := "/usr/local/go/bin/go"
	if _, err := os.Stat(goBin); err != nil {
		goBin = "go"
	}
	goCmd := exec.Command(goBin, "build", "-ldflags=-w -s", "-o", "agent-bridge", ".")
	goCmd.Dir = dir
	goCmd.Env = append(os.Environ(), "PATH="+envPath, "TMPDIR="+tmpDir)
	var goBuf bytes.Buffer
	goCmd.Stdout = &goBuf
	goCmd.Stderr = &goBuf
	if err := goCmd.Run(); err != nil {
		fail("Build Go Binary", fmt.Errorf("%s: %s", err, goBuf.String()))
		return
	}
	u.logStep("Đã biên dịch thành công nhị phân ./agent-bridge.")

	// Step 4: Refresh metadata
	newCommit, _ := runGitCmd(context.Background(), dir, "rev-parse", "--short", "HEAD")
	newMsg, _ := runGitCmd(context.Background(), dir, "log", "-1", "--format=%s")
	rawNewTag, _ := runGitCmd(context.Background(), dir, "describe", "--tags", "--abbrev=0")
	newBuildStr, _ := runGitCmd(context.Background(), dir, "rev-list", "--count", "HEAD")
	newBuild, _ := strconv.Atoi(strings.TrimSpace(newBuildStr))
	newVersion := formatVersion(rawNewTag, newBuild)

	u.mu.Lock()
	u.status.CurrentCommit = newCommit
	u.status.CurrentMessage = newMsg
	u.status.CurrentVersion = newVersion
	u.status.CurrentBuild = newBuild
	u.status.LatestVersion = newVersion
	u.status.LatestBuild = newBuild
	u.status.HasUpdate = false
	u.status.CommitsBehind = 0
	u.status.Commits = []string{}
	u.status.Step = "restarting"
	u.status.Logs = append(u.status.Logs, fmt.Sprintf("[%s] ✅ Cập nhật thành công lên %s! Đang khởi động lại dịch vụ...", time.Now().Format("15:04:05"), newVersion))
	u.mu.Unlock()

	// Step 5: Restart service after slight delay so UI captures the "restarting" status
	go func() {
		time.Sleep(1500 * time.Millisecond)
		log.Println("[update] executing restart...")

		// 1. Try systemctl restart
		cmd := exec.Command("sudo", "systemctl", "restart", "agent-bridge.service")
		if err := cmd.Run(); err == nil {
			return
		}

		// 2. Fallback to in-place exec
		exe, err := os.Executable()
		if err == nil {
			_ = syscall.Exec(exe, os.Args, os.Environ())
		}
	}()
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

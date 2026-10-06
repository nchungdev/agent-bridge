package server

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ApplyUpdate executes git pull, web build, Go compile, and service restart.
func (u *UpdateManager) ApplyUpdate() error {
	mode := updateMode()
	if mode == "off" {
		return fmt.Errorf("cập nhật tại chỗ đã tắt (AGENT_BRIDGE_UPDATE=off)")
	}
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

	if mode == "release" {
		go u.runReleasePipeline()
	} else {
		go u.runPipeline()
	}
	return nil
}

func (u *UpdateManager) logStep(msg string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	u.status.Logs = append(u.status.Logs, entry)
	log.Printf("[update] %s", msg)
}

func buildBuildEnvPath() string {
	extra := []string{"/usr/local/go/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		extra = append(extra, filepath.Join(home, "go", "bin"))
	}
	return strings.Join(extra, string(filepath.ListSeparator)) + string(filepath.ListSeparator) + os.Getenv("PATH")
}

func (u *UpdateManager) runPipeline() {
	dir := getRepoDir()
	envPath := buildBuildEnvPath()
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

	goBin, err := exec.LookPath("go")
	if err != nil {
		if _, statErr := os.Stat("/usr/local/go/bin/go"); statErr == nil {
			goBin = "/usr/local/go/bin/go"
		} else {
			goBin = "go"
		}
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
	rawNewTag, newBuild := versionAt(context.Background(), dir, "HEAD")
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
	go restartService()
}

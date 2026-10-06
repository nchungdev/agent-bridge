package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/nchungdev/agent-bridge/internal/release"
	"github.com/nchungdev/agent-bridge/internal/service"
	"github.com/nchungdev/agent-bridge/internal/update"
	"github.com/nchungdev/agent-bridge/internal/version"
)

// updateMode says how this copy updates itself:
//
//	release  download the newest build from GitHub Releases (the default for an installed release binary)
//	git      pull and rebuild a checkout (the default for a development build)
//	off      never (set in the container image: update by pulling a new image)
//
// AGENT_BRIDGE_UPDATE overrides the default.
func updateMode() string {
	switch m := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_BRIDGE_UPDATE"))); m {
	case "release", "git", "off":
		return m
	}
	if version.Installed() {
		return "release"
	}
	return "git"
}

// checkRelease fills the status from the embedded version and GitHub Releases. The network is only asked when
// fetch is set or nothing was fetched yet, so the status endpoint can be polled cheaply.
func (u *UpdateManager) checkRelease(ctx context.Context, fetch bool) (*UpdateStatus, error) {
	u.mu.Lock()
	need := updateMode() == "release" && (fetch || u.fetched.IsZero())
	u.mu.Unlock()

	var latest *update.Release
	var ferr error
	if need {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		latest, ferr = update.New().Latest(cctx)
		cancel()
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	if need {
		u.fetched = time.Now() // also after a failure, so an offline machine is not retried on every poll
		if ferr == nil {
			u.latest = latest
		}
	}

	cur := version.Version
	_, curBuild, _ := release.Parse(cur)
	st := &u.status
	st.CurrentVersion = release.Display(cur)
	st.CurrentBuild = curBuild
	st.CurrentCommit = version.Commit
	if len(st.CurrentCommit) > 7 {
		st.CurrentCommit = st.CurrentCommit[:7]
	}
	st.CurrentMessage, st.CurrentDate, st.Branch, st.RemoteCommit = "", "", "", ""
	st.CommitsBehind, st.Commits = 0, []string{}
	st.HasUpdate = u.latest != nil && release.Newer(u.latest.Tag, cur)
	st.LatestVersion, st.LatestBuild = st.CurrentVersion, curBuild
	if st.HasUpdate {
		_, st.LatestBuild, _ = release.Parse(u.latest.Tag)
		st.LatestVersion = release.Display(u.latest.Tag)
	}
	if need && ferr == nil {
		st.LastChecked = u.fetched.Format(time.RFC3339)
	}

	if ferr != nil && fetch {
		return nil, fmt.Errorf("không kiểm tra được bản phát hành: %w", ferr)
	}
	cp := *st
	return &cp, nil
}

// fail ends an update run with an error.
func (u *UpdateManager) fail(step string, err error) {
	u.mu.Lock()
	u.status.IsUpdating = false
	u.status.Step = "error"
	u.status.Error = fmt.Sprintf("%s: %v", step, err)
	u.status.Logs = append(u.status.Logs, fmt.Sprintf("[%s] ❌ Lỗi tại bước %s: %v", time.Now().Format("15:04:05"), step, err))
	u.mu.Unlock()
	log.Printf("[update] error in %s: %v", step, err)
}

// runReleasePipeline downloads the newest release, verifies and installs it, and restarts.
func (u *UpdateManager) runReleasePipeline() {
	u.logStep("Đang tìm bản phát hành mới nhất trên GitHub...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	up := update.New()
	latest, err := up.Latest(ctx)
	if err != nil {
		u.fail("Tìm bản phát hành", err)
		return
	}
	if latest == nil || !release.Newer(latest.Tag, version.Version) {
		u.mu.Lock()
		u.status.IsUpdating = false
		u.status.Step = "idle"
		u.status.Logs = append(u.status.Logs, fmt.Sprintf("[%s] Đang dùng bản mới nhất.", time.Now().Format("15:04:05")))
		u.mu.Unlock()
		return
	}

	exe, err := update.Executable()
	if err != nil {
		u.fail("Tìm file chạy", err)
		return
	}
	u.logStep(fmt.Sprintf("Đang tải và kiểm tra %s...", release.Display(latest.Tag)))
	if err := up.Apply(ctx, latest, exe); err != nil {
		u.fail("Cài bản mới", err)
		return
	}

	u.mu.Lock()
	u.latest = latest
	u.status.CurrentVersion = release.Display(latest.Tag)
	_, u.status.CurrentBuild, _ = release.Parse(latest.Tag)
	u.status.LatestVersion, u.status.LatestBuild = u.status.CurrentVersion, u.status.CurrentBuild
	u.status.HasUpdate = false
	u.status.Step = "restarting"
	u.status.Logs = append(u.status.Logs, fmt.Sprintf("[%s] ✅ Đã cài %s. Đang khởi động lại dịch vụ...", time.Now().Format("15:04:05"), u.status.CurrentVersion))
	u.mu.Unlock()

	go restartService()
}

// restartService restarts this server after an update, after a short delay so the UI sees the "restarting"
// step: through AGENT_BRIDGE_RESTART_CMD if set, else the service manager, else by re-executing itself.
func restartService() {
	time.Sleep(1500 * time.Millisecond)
	log.Println("[update] executing restart...")

	if customCmd := os.Getenv("AGENT_BRIDGE_RESTART_CMD"); customCmd != "" {
		if parts := strings.Fields(customCmd); len(parts) > 0 {
			if err := exec.Command(parts[0], parts[1:]...).Run(); err == nil {
				return
			}
		}
	}
	if err := service.Restart(); err == nil {
		return
	}
	exe, err := update.Executable()
	if err != nil {
		log.Printf("[update] restart: %v", err)
		return
	}
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		log.Printf("[update] restart: %v", err)
	}
}

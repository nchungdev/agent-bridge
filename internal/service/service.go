// Package service installs Agent Bridge as a background service that starts at login or boot and is restarted
// if it dies: a systemd unit on Linux (per user, or system-wide with Options.System), a launchd agent on macOS.
package service

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Label identifies the service to launchd (and names the systemd unit).
const (
	Label    = "com.nchungdev.agent-bridge"
	UnitName = "agent-bridge.service"
)

// Options describes the service to install.
type Options struct {
	Exe     string // absolute path of the agent-bridge binary
	Home    string // home directory of the account that runs it
	Host    string // listen address
	Port    int
	DataDir string // state directory (database, terminals' tmux socket, uploads)
	System  bool   // Linux only: a system-wide unit (needs root) instead of a per-user one
	User    string // the account a system-wide unit runs as
	Force   bool   // overwrite a service file that differs from the one generated here
	GOOS    string // platform to install for (this machine's by default); tests set it
}

func (o *Options) fill() error {
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.Home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		o.Home = h
	}
	if o.Exe == "" {
		return errors.New("the path of the binary is required")
	}
	if o.Host == "" {
		o.Host = "127.0.0.1"
	}
	if o.Port == 0 {
		o.Port = 8088
	}
	if o.DataDir == "" {
		o.DataDir = filepath.Join(o.Home, ".agent-bridge")
	}
	return nil
}

// EnvFile is the optional file where the user keeps secrets and overrides (AGENT_BRIDGE_TOKEN=...), so
// they never have to edit the service file itself.
func (o Options) EnvFile() string { return filepath.Join(o.DataDir, "agent-bridge.env") }

// path lists the places agent CLIs are usually installed: a service starts with a minimal PATH.
func (o Options) path() string {
	return strings.Join([]string{
		filepath.Join(o.Home, ".local", "bin"),
		filepath.Join(o.Home, ".npm-global", "bin"),
		"/home/linuxbrew/.linuxbrew/bin",
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
	}, ":")
}

// UnitFile is the systemd unit.
func UnitFile(o Options) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "[Unit]\nDescription=Agent Bridge: switch between Claude Code, Codex and Antigravity without losing context\nAfter=network.target\n\n")
	fmt.Fprintf(&b, "[Service]\nType=simple\n")
	if o.System && o.User != "" {
		fmt.Fprintf(&b, "User=%s\n", o.User)
	}
	fmt.Fprintf(&b, "ExecStart=\"%s\"\nWorkingDirectory=%s\n", o.Exe, o.Home) // quoted: paths may contain spaces
	for _, kv := range [][2]string{
		{"AGENT_BRIDGE_HOST", o.Host},
		{"AGENT_BRIDGE_PORT", strconv.Itoa(o.Port)},
		{"AGENT_BRIDGE_DATA_DIR", o.DataDir},
		{"AGENT_BRIDGE_V2", "1"},
		{"AGENT_BRIDGE_MAX_LIVE", "2"},
		{"PATH", o.path()},
	} {
		fmt.Fprintf(&b, "Environment=\"%s=%s\"\n", kv[0], kv[1])
	}
	fmt.Fprintf(&b, "EnvironmentFile=-%s\n", o.EnvFile())
	fmt.Fprintf(&b, "Restart=always\nRestartSec=3\n")
	fmt.Fprintf(&b, "# restarting must not end the terminals and agents running in tmux\nKillMode=process\n\n")
	target := "default.target"
	if o.System {
		target = "multi-user.target"
	}
	fmt.Fprintf(&b, "[Install]\nWantedBy=%s\n", target)
	return b.String()
}

// Plist is the launchd agent.
func Plist(o Options) string {
	esc := html.EscapeString
	logFile := filepath.Join(o.Home, "Library", "Logs", "agent-bridge.log")
	var b bytes.Buffer
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key><array><string>%s</string></array>
  <key>WorkingDirectory</key><string>%s</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
`, Label, esc(o.Exe), esc(o.Home), esc(logFile), esc(logFile))
	for _, kv := range [][2]string{
		{"AGENT_BRIDGE_HOST", o.Host},
		{"AGENT_BRIDGE_PORT", strconv.Itoa(o.Port)},
		{"AGENT_BRIDGE_DATA_DIR", o.DataDir},
		{"AGENT_BRIDGE_V2", "1"},
		{"AGENT_BRIDGE_MAX_LIVE", "2"},
		{"PATH", o.path()},
	} {
		fmt.Fprintf(&b, "    <key>%s</key><string>%s</string>\n", kv[0], esc(kv[1]))
	}
	fmt.Fprintf(&b, "  </dict>\n</dict>\n</plist>\n")
	return b.String()
}

// run executes a command; tests replace it.
var run = func(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// File is where the service definition lives.
func File(o Options) (string, error) {
	if err := o.fill(); err != nil {
		return "", err
	}
	switch o.GOOS {
	case "linux":
		if o.System {
			return "/etc/systemd/system/" + UnitName, nil
		}
		return filepath.Join(o.Home, ".config", "systemd", "user", UnitName), nil
	case "darwin":
		return filepath.Join(o.Home, "Library", "LaunchAgents", Label+".plist"), nil
	}
	return "", fmt.Errorf("installing as a service is not supported on %s yet", o.GOOS)
}

func (o Options) systemctl(args ...string) error {
	if o.System {
		return run("systemctl", args...)
	}
	return run("systemctl", append([]string{"--user"}, args...)...)
}

func (o Options) domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

// Install writes the service definition and starts the service. It refuses to overwrite a service file that
// differs from the one generated here (for example one the user wrote by hand) unless Options.Force is set.
func Install(o Options) error {
	if err := o.fill(); err != nil {
		return err
	}
	file, err := File(o)
	if err != nil {
		return err
	}
	content := UnitFile(o)
	if o.GOOS == "darwin" {
		content = Plist(o)
	}
	if existing, err := os.ReadFile(file); err == nil && string(existing) != content && !o.Force {
		return fmt.Errorf("%s already exists and is different: it is probably a service you set up yourself.\n"+
			"Re-run with --force to replace it (the old file is kept as %s.bak)", file, file)
	}
	if existing, err := os.ReadFile(file); err == nil && string(existing) != content {
		_ = os.WriteFile(file+".bak", existing, 0o644)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(o.DataDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return err
	}
	if o.GOOS == "darwin" {
		_ = os.MkdirAll(filepath.Join(o.Home, "Library", "Logs"), 0o755)
		_ = run("launchctl", "bootout", o.domain()+"/"+Label) // not loaded yet is fine
		if err := run("launchctl", "bootstrap", o.domain(), file); err != nil {
			return err
		}
		return run("launchctl", "kickstart", "-k", o.domain()+"/"+Label)
	}
	if err := o.systemctl("daemon-reload"); err != nil {
		return err
	}
	if err := o.systemctl("enable", "--now", UnitName); err != nil {
		return err
	}
	if !o.System {
		// let a per-user service keep running after logout and start at boot (best effort)
		if u := os.Getenv("USER"); u != "" {
			_ = run("loginctl", "enable-linger", u)
		}
	}
	return nil
}

// Uninstall stops the service and removes its definition. Data is left alone.
func Uninstall(o Options) error {
	if err := o.fill(); err != nil {
		return err
	}
	file, err := File(o)
	if err != nil {
		return err
	}
	if o.GOOS == "darwin" {
		_ = run("launchctl", "bootout", o.domain()+"/"+Label)
	} else {
		_ = o.systemctl("disable", "--now", UnitName)
	}
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return err
	}
	if o.GOOS == "linux" {
		_ = o.systemctl("daemon-reload")
	}
	return nil
}

// Status is a human-readable description of the service.
func Status(o Options) (string, error) {
	if err := o.fill(); err != nil {
		return "", err
	}
	if o.GOOS == "darwin" {
		out, err := exec.Command("launchctl", "print", o.domain()+"/"+Label).CombinedOutput()
		return string(out), err
	}
	args := []string{"status", "--no-pager", UnitName}
	if !o.System {
		args = append([]string{"--user"}, args...)
	}
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	return string(out), err
}

// Restart restarts the service through whatever manages it here and reports an error when none does, so the
// caller can fall back to re-executing itself.
func Restart() error {
	switch runtime.GOOS {
	case "darwin":
		return run("launchctl", "kickstart", "-k", "gui/"+strconv.Itoa(os.Getuid())+"/"+Label)
	case "linux":
		if err := exec.Command("systemctl", "--user", "is-active", "--quiet", UnitName).Run(); err == nil {
			return run("systemctl", "--user", "restart", UnitName)
		}
		if err := exec.Command("systemctl", "is-active", "--quiet", UnitName).Run(); err == nil {
			return run("sudo", "-n", "systemctl", "restart", UnitName)
		}
	}
	return errors.New("not running as a managed service")
}

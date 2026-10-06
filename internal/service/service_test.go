package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func opts(t *testing.T, goos string) Options {
	t.Helper()
	home := t.TempDir()
	return Options{Exe: "/home/u/.local/bin/agent-bridge", Home: home, DataDir: filepath.Join(home, "data"), Port: 8088, Host: "127.0.0.1", GOOS: goos}
}

// capture replaces the command runner for a test and returns what was run.
func capture(t *testing.T) *[]string {
	t.Helper()
	var cmds []string
	old := run
	run = func(name string, args ...string) error {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		return nil
	}
	t.Cleanup(func() { run = old })
	return &cmds
}

func has(cmds []string, want string) bool {
	for _, c := range cmds {
		if c == want {
			return true
		}
	}
	return false
}

func TestUnitFile(t *testing.T) {
	o := opts(t, "linux")
	u := UnitFile(o)
	for _, want := range []string{
		`ExecStart="/home/u/.local/bin/agent-bridge"`,
		`Environment="AGENT_BRIDGE_PORT=8088"`,
		`Environment="AGENT_BRIDGE_HOST=127.0.0.1"`,
		`Environment="AGENT_BRIDGE_V2=1"`,
		"EnvironmentFile=-" + o.EnvFile(),
		"Restart=always",
		"KillMode=process", // restarting the service must not end the terminals
		"WantedBy=default.target",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("unit file lacks %q:\n%s", want, u)
		}
	}
	if strings.Contains(u, "User=") {
		t.Error("a per-user unit must not name a User=")
	}

	o.System, o.User = true, "bob"
	sys := UnitFile(o)
	if !strings.Contains(sys, "User=bob") || !strings.Contains(sys, "WantedBy=multi-user.target") {
		t.Errorf("system unit must run as the user and start at boot:\n%s", sys)
	}
}

func TestUnitFileQuotesPathsWithSpaces(t *testing.T) {
	o := opts(t, "linux")
	o.Exe = "/home/u/AI Workspace/bin/agent-bridge"
	if u := UnitFile(o); !strings.Contains(u, `ExecStart="/home/u/AI Workspace/bin/agent-bridge"`) {
		t.Errorf("a path with a space must stay quoted:\n%s", u)
	}
}

func TestPlist(t *testing.T) {
	o := opts(t, "darwin")
	o.Exe = "/Users/a&b/agent-bridge"
	p := Plist(o)
	for _, want := range []string{
		"<string>" + Label + "</string>",
		"<string>/Users/a&amp;b/agent-bridge</string>", // escaped for XML
		"<key>KeepAlive</key><true/>",
		"<key>AGENT_BRIDGE_PORT</key><string>8088</string>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist lacks %q:\n%s", want, p)
		}
	}
}

func TestInstallLinuxUserService(t *testing.T) {
	cmds := capture(t)
	o := opts(t, "linux")
	if err := Install(o); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(o.Home, ".config", "systemd", "user", UnitName)
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != UnitFile(o) {
		t.Errorf("unit file was not written as generated")
	}
	for _, want := range []string{"systemctl --user daemon-reload", "systemctl --user enable --now " + UnitName} {
		if !has(*cmds, want) {
			t.Errorf("missing command %q in %v", want, *cmds)
		}
	}
	if st, err := os.Stat(o.DataDir); err != nil || !st.IsDir() {
		t.Error("the data directory must be created")
	}
}

func TestInstallRefusesToOverwriteADifferentServiceFile(t *testing.T) {
	capture(t)
	o := opts(t, "linux")
	file := filepath.Join(o.Home, ".config", "systemd", "user", UnitName)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("[Service]\nExecStart=/mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Install(o)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("Install error = %v, want a refusal that mentions --force", err)
	}
	if b, _ := os.ReadFile(file); string(b) != "[Service]\nExecStart=/mine\n" {
		t.Error("the user's own service file must be left alone")
	}

	o.Force = true
	if err := Install(o); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file + ".bak"); string(b) != "[Service]\nExecStart=/mine\n" {
		t.Error("with --force the old file is kept as .bak")
	}
	if b, _ := os.ReadFile(file); string(b) != UnitFile(o) {
		t.Error("with --force the generated unit replaces the old one")
	}
}

func TestInstallDarwinLaunchAgent(t *testing.T) {
	cmds := capture(t)
	o := opts(t, "darwin")
	if err := Install(o); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(o.Home, "Library", "LaunchAgents", Label+".plist")
	if b, err := os.ReadFile(file); err != nil || string(b) != Plist(o) {
		t.Fatalf("plist not written as generated: %v", err)
	}
	joined := strings.Join(*cmds, "\n")
	for _, want := range []string{"launchctl bootout gui/", "launchctl bootstrap gui/", "launchctl kickstart -k gui/"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
}

func TestUninstall(t *testing.T) {
	cmds := capture(t)
	o := opts(t, "linux")
	if err := Install(o); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(o); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(o.Home, ".config", "systemd", "user", UnitName)
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("the unit file must be removed")
	}
	if !has(*cmds, "systemctl --user disable --now "+UnitName) {
		t.Errorf("the service must be stopped: %v", *cmds)
	}
	if st, err := os.Stat(o.DataDir); err != nil || !st.IsDir() {
		t.Error("uninstalling must leave the user's data alone")
	}
	if err := Uninstall(o); err != nil {
		t.Errorf("uninstalling twice must not fail: %v", err)
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	if _, err := File(opts(t, "windows")); err == nil {
		t.Error("windows is not supported yet and must say so")
	}
}

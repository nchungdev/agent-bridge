package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/user"
	"time"

	"github.com/nchungdev/agent-bridge/internal/release"
	"github.com/nchungdev/agent-bridge/internal/service"
	"github.com/nchungdev/agent-bridge/internal/update"
	"github.com/nchungdev/agent-bridge/internal/version"
)

const usageText = `Agent Bridge

Usage:
  agent-bridge                        run the server
  agent-bridge version                print the installed version
  agent-bridge update [--check]       update to the newest release from GitHub (then restart the service)
  agent-bridge update --rollback      go back to the version that was installed before the last update
  agent-bridge service install        run Agent Bridge in the background and start it at login/boot
  agent-bridge service uninstall      stop and remove that service (your data is kept)
  agent-bridge service status|restart
  agent-bridge nas report [--zalo]    print the machine report (CPU, RAM, disks, temperatures), or send it to Zalo

Service options: --port N (8088)  --host ADDR (127.0.0.1)  --data-dir DIR  --system (Linux, needs root)
                 --user NAME (with --system)  --force (replace a service file you made yourself)
`

func versionLine() string {
	line := "agent-bridge " + release.Display(version.Version)
	if version.Commit != "" {
		line += " (" + version.Commit + ")"
	}
	return line
}

// runCLI handles the subcommands. It returns false when there are none, and the server should start.
func runCLI(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Println(versionLine())
	case "update":
		os.Exit(cmdUpdate(args[1:]))
	case "service":
		os.Exit(cmdService(args[1:]))
	case "nas":
		os.Exit(cmdNAS(args[1:]))
	case "help", "--help", "-h":
		fmt.Print(usageText)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", args[0], usageText)
		os.Exit(2)
	}
	return true
}

func cmdUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	check := fs.Bool("check", false, "only report whether a newer release exists")
	rollback := fs.Bool("rollback", false, "go back to the previous version")
	noRestart := fs.Bool("no-restart", false, "do not restart the service afterwards")
	force := fs.Bool("force", false, "update even though this is a development build")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	exe, err := update.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot locate the running binary:", err)
		return 1
	}

	restart := func() {
		if *noRestart {
			fmt.Println("Restart Agent Bridge to run the new version.")
			return
		}
		if err := service.Restart(); err != nil {
			fmt.Println("Restart Agent Bridge yourself to run the new version (it is not running as a managed service here).")
			return
		}
		fmt.Println("Restarted. Terminals and agents that were running in tmux are still there.")
	}

	if *rollback {
		if err := update.Rollback(exe); err != nil {
			fmt.Fprintln(os.Stderr, "rollback failed:", err)
			return 1
		}
		fmt.Println("Went back to the previous version.")
		restart()
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	u := update.New()
	latest, err := u.Latest(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot reach GitHub releases:", err)
		return 1
	}
	fmt.Println("installed:", release.Display(version.Version))
	if latest == nil {
		fmt.Println("latest:    none published for this platform")
		return 0
	}
	fmt.Println("latest:   ", release.Display(latest.Tag))
	if !release.Newer(latest.Tag, version.Version) {
		fmt.Println("Already up to date.")
		return 0
	}
	if *check {
		fmt.Println("An update is available: run `agent-bridge update`.")
		return 0
	}
	if !version.Installed() && !*force {
		fmt.Println("This is a development build (run from a checkout): update it with git, or pass --force to replace it with the release.")
		return 1
	}
	fmt.Println("Downloading and verifying", latest.Tag, "...")
	if err := u.Apply(ctx, latest, exe); err != nil {
		fmt.Fprintln(os.Stderr, "update failed, nothing was changed:", err)
		return 1
	}
	fmt.Println("Updated to", release.Display(latest.Tag)+". The previous version is kept: `agent-bridge update --rollback` goes back.")
	restart()
	return 0
}

func cmdService(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
	sub := args[0]
	fs := flag.NewFlagSet("service "+sub, flag.ContinueOnError)
	port := fs.Int("port", 8088, "listen port")
	host := fs.String("host", "127.0.0.1", "listen address (a non-loopback address requires AGENT_BRIDGE_TOKEN)")
	data := fs.String("data-dir", "", "state directory (default ~/.agent-bridge)")
	system := fs.Bool("system", false, "Linux: install a system-wide service (needs root)")
	account := fs.String("user", "", "account the system-wide service runs as")
	force := fs.Bool("force", false, "replace a service file that differs from the generated one")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	exe, err := update.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot locate the running binary:", err)
		return 1
	}
	o := service.Options{Exe: exe, Host: *host, Port: *port, DataDir: *data, System: *system, User: *account, Force: *force}
	if o.System {
		name := o.User
		if name == "" {
			name = os.Getenv("SUDO_USER")
		}
		var u *user.User
		if name != "" {
			u, err = user.Lookup(name)
		} else {
			u, err = user.Current()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot find the account to run as:", err)
			return 1
		}
		o.User, o.Home = u.Username, u.HomeDir
	} else if os.Geteuid() == 0 {
		fmt.Fprintln(os.Stderr, "warning: running as root without --system installs the service for root")
	}

	switch sub {
	case "install":
		if err := service.Install(o); err != nil {
			fmt.Fprintln(os.Stderr, "install failed:", err)
			return 1
		}
		file, _ := service.File(o)
		fmt.Printf("Agent Bridge is running as a service.\n  open:   http://%s:%d\n  file:   %s\n", *host, *port, file)
		fmt.Printf("  config: %s (optional; for example AGENT_BRIDGE_TOKEN=...)\n", o.EnvFile())
		fmt.Println("  status: agent-bridge service status")
	case "uninstall":
		if err := service.Uninstall(o); err != nil {
			fmt.Fprintln(os.Stderr, "uninstall failed:", err)
			return 1
		}
		fmt.Println("Service removed. Your data and the binary were left in place.")
	case "status":
		out, err := service.Status(o)
		fmt.Print(out)
		if err != nil {
			return 1
		}
	case "restart":
		if err := service.Restart(); err != nil {
			fmt.Fprintln(os.Stderr, "restart failed:", err)
			return 1
		}
		fmt.Println("Restarted.")
	default:
		fmt.Fprintf(os.Stderr, "unknown service command %q\n\n%s", sub, usageText)
		return 2
	}
	return 0
}

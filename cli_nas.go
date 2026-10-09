package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nchungdev/agent-bridge/internal/nas"
	"github.com/nchungdev/agent-bridge/internal/zalo"
)

const nasUsage = "usage: agent-bridge nas report|alerts [--zalo] [--env-file FILE] [--state-file FILE] [--quiet HH:MM-HH:MM]\n"

// nasFlags are the options `nas report` and `nas alerts` share.
type nasFlags struct {
	zalo      bool
	envFile   string
	stateFile string
	quiet     string
}

func parseNASFlags(name string, args []string) (nasFlags, bool) {
	var f nasFlags
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.BoolVar(&f.zalo, "zalo", false, "send to Zalo instead of printing (needs ZALO_BOT_TOKEN and ZALO_CHAT_ID)")
	fs.StringVar(&f.envFile, "env-file", "", "file with NAME=value lines to read before anything else")
	fs.StringVar(&f.stateFile, "state-file", "", "alerts only: remember what was sent here, so a lasting problem is not repeated on every run")
	fs.StringVar(&f.quiet, "quiet", "", "alerts only: quiet hours as HH:MM-HH:MM (local time); only urgent alerts (heat, a failing drive) are sent then")
	return f, fs.Parse(args) == nil
}

// cmdNAS implements `agent-bridge nas report|alerts`: the machine report of the /nas chat command, and the
// list of problems, printed or sent to Zalo, for use from cron or an OpenMediaVault scheduled task.
func cmdNAS(args []string) int {
	if len(args) == 0 || (args[0] != "report" && args[0] != "alerts") {
		fmt.Fprint(os.Stderr, nasUsage)
		return 2
	}
	f, ok := parseNASFlags("nas "+args[0], args[1:])
	if !ok {
		return 2
	}
	if f.envFile != "" {
		if err := loadEnvFile(f.envFile); err != nil {
			return fail(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	host := nas.New(nas.ExecRunner{}, nil)
	var err error
	if args[0] == "report" {
		err = nasReport(ctx, host, f)
	} else {
		err = nasAlerts(ctx, host, f)
	}
	if err != nil {
		return fail(err)
	}
	return 0
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "agent-bridge:", err)
	return 1
}

func nasReport(ctx context.Context, host *nas.Host, f nasFlags) error {
	report, err := host.Overview(ctx)
	if err != nil {
		return err
	}
	return deliver(ctx, f, report)
}

// nasAlerts prints every current alert, or with --zalo sends only those that are due (see nas.Throttle).
func nasAlerts(ctx context.Context, host *nas.Host, f nasFlags) error {
	alerts, err := host.Alerts(ctx)
	if err != nil && len(alerts) == 0 {
		return err
	}
	if !f.zalo {
		for _, a := range alerts {
			fmt.Println(a.Text)
		}
		if len(alerts) == 0 {
			fmt.Println("Không có cảnh báo.")
		}
		return nil
	}
	quiet, err := nas.ParseQuietHours(f.quiet)
	if err != nil {
		return err
	}
	muted := func(a nas.Alert) bool { return quiet.Contains(time.Now()) && !a.Urgent }
	due, commit := nas.NewThrottle(f.stateFile).Select(alerts, muted)
	if len(due) > 0 {
		lines := make([]string, 0, len(due)+1)
		lines = append(lines, "⚠️ Cảnh báo NAS:")
		for _, a := range due {
			lines = append(lines, a.Text)
		}
		if err := deliver(ctx, f, strings.Join(lines, "\n")); err != nil {
			return err // nothing is remembered, so the next run tries again
		}
	}
	return commit()
}

// deliver prints text, or sends it to the Zalo chat named in the environment.
func deliver(ctx context.Context, f nasFlags, text string) error {
	if !f.zalo {
		fmt.Println(text)
		return nil
	}
	token, chatID := os.Getenv("ZALO_BOT_TOKEN"), os.Getenv("ZALO_CHAT_ID")
	if token == "" || chatID == "" {
		return errors.New("ZALO_BOT_TOKEN and ZALO_CHAT_ID must be set")
	}
	return zalo.NewClient(zalo.Config{Token: token, APIBase: zalo.DefaultAPIBase}).SendText(ctx, chatID, text)
}

// loadEnvFile sets the NAME=value lines of a file in the environment (blank lines and # comments are skipped).
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if name, value, ok := strings.Cut(line, "="); ok && line != "" && !strings.HasPrefix(line, "#") {
			if err := os.Setenv(strings.TrimSpace(name), strings.Trim(strings.TrimSpace(value), `"'`)); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

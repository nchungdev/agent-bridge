package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nchungdev/agent-bridge/internal/nas"
	"github.com/nchungdev/agent-bridge/internal/zalo"
)

// cmdNAS implements `agent-bridge nas report [--zalo --env-file FILE]`: the same machine report as the /nas
// chat command, printed or sent to Zalo, for use from cron.
func cmdNAS(args []string) int {
	if len(args) == 0 || args[0] != "report" {
		fmt.Fprint(os.Stderr, "usage: agent-bridge nas report [--zalo] [--env-file FILE]\n")
		return 2
	}
	fs := flag.NewFlagSet("nas report", flag.ContinueOnError)
	toZalo := fs.Bool("zalo", false, "send the report to Zalo instead of printing it (needs ZALO_BOT_TOKEN and ZALO_CHAT_ID)")
	envFile := fs.String("env-file", "", "file with NAME=value lines to read before anything else")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *envFile != "" {
		if err := loadEnvFile(*envFile); err != nil {
			fmt.Fprintln(os.Stderr, "agent-bridge:", err)
			return 1
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := nas.New(nas.ExecRunner{}, nil).Overview(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-bridge:", err)
		return 1
	}
	if !*toZalo {
		fmt.Println(report)
		return 0
	}
	token, chatID := os.Getenv("ZALO_BOT_TOKEN"), os.Getenv("ZALO_CHAT_ID")
	if token == "" || chatID == "" {
		fmt.Fprintln(os.Stderr, "agent-bridge: ZALO_BOT_TOKEN and ZALO_CHAT_ID must be set")
		return 1
	}
	if err := zalo.NewClient(zalo.Config{Token: token, APIBase: zalo.DefaultAPIBase}).SendText(ctx, chatID, report); err != nil {
		fmt.Fprintln(os.Stderr, "agent-bridge:", err)
		return 1
	}
	return 0
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

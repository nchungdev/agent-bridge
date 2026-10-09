// Package nas reports on and controls the machine Agent Bridge runs on: CPU, memory, disks, and the Docker
// containers and systemd units installed on it. Everything runs as direct commands, never through an agent,
// and every command is started without a shell with fixed arguments, so no text a user sends can become a
// command line.
package nas

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Runner starts a program and returns what it printed on stdout. A non-nil error may come with useful output
// (smartctl, for instance, reports findings through its exit code).
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// ExecRunner runs real programs.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s: %s", name, firstLine(msg))
	}
	return stdout.String(), nil
}

// system is how Host reads the kernel's files and filesystem figures; tests replace it.
type system struct {
	readFile func(path string) (string, error)
	glob     func(pattern string) ([]string, error)
	statfs   func(path string) (total, free uint64, err error)
	hostname func() (string, error)
	sleep    func(time.Duration)
	cores    func() int
}

// Host is the machine.
type Host struct {
	run   Runner
	units []string // systemd units that may be listed and controlled (an explicit allow-list)
	sys   system
}

// New returns a Host. units are the systemd units to expose besides the Docker containers.
func New(run Runner, units []string) *Host {
	return &Host{run: run, units: units, sys: realSystem()}
}

// Errors a caller can show to the user.
var (
	ErrUnknownService = errors.New("không có service này (xem /nas service-list)")
	ErrUnsupported    = errors.New("thao tác này không áp dụng được cho service đó")
)

// Commands that may take a while get their own limits.
const (
	quickTimeout  = 15 * time.Second
	actionTimeout = 90 * time.Second
	pullTimeout   = 10 * time.Minute
)

// exec runs a program with a time limit.
func (h *Host) exec(ctx context.Context, limit time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	return h.run.Run(ctx, name, args...)
}

// sudo runs a program as root through passwordless sudo (smartctl, systemctl).
func (h *Host) sudo(ctx context.Context, limit time.Duration, name string, args ...string) (string, error) {
	return h.exec(ctx, limit, "sudo", append([]string{"-n", name}, args...)...)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func realSystem() system {
	return system{
		readFile: func(p string) (string, error) { b, err := os.ReadFile(p); return string(b), err },
		glob:     globFiles,
		statfs:   statfs,
		hostname: os.Hostname,
		sleep:    time.Sleep,
		cores:    numCPU,
	}
}

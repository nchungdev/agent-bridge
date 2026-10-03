// Package proc is the stdio transport shared by CLI adapters: a child process
// in its own process group with a line-oriented JSON channel.
package proc

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Proc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	Stdout *bufio.Reader
	done   chan struct{}

	wmu sync.Mutex

	emu    sync.Mutex
	stderr []string
}

type Options struct {
	Bin  string
	Args []string
	Dir  string
	Env  []string
}

func Start(ctx context.Context, o Options) (*Proc, error) {
	cmd := exec.CommandContext(ctx, o.Bin, o.Args...)
	cmd.Dir = o.Dir
	cmd.Env = append(os.Environ(), o.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn %s: %w", o.Bin, err)
	}
	p := &Proc{cmd: cmd, stdin: in, Stdout: bufio.NewReaderSize(out, 1<<20), done: make(chan struct{})}
	go func() {
		sc := bufio.NewScanner(errPipe)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			p.emu.Lock()
			p.stderr = append(p.stderr, sc.Text())
			if len(p.stderr) > 20 {
				p.stderr = p.stderr[1:]
			}
			p.emu.Unlock()
		}
	}()
	go func() { _ = cmd.Wait(); close(p.done) }()
	return p, nil
}

// WriteLine writes one line (newline appended) to the child's stdin.
func (p *Proc) WriteLine(b []byte) error {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if _, err := p.stdin.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// StderrTail returns the last stderr lines (for error reporting).
func (p *Proc) StderrTail() string {
	p.emu.Lock()
	defer p.emu.Unlock()
	return strings.Join(p.stderr, "\n")
}

func (p *Proc) Done() <-chan struct{} { return p.done }

// Close ends the process: close stdin, wait briefly, then SIGTERM, then SIGKILL the group.
func (p *Proc) Close() {
	_ = p.stdin.Close()
	select {
	case <-p.done:
		return
	case <-time.After(1500 * time.Millisecond):
	}
	if p.cmd.Process != nil {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
	}
	select {
	case <-p.done:
		return
	case <-time.After(2 * time.Second):
	}
	if p.cmd.Process != nil {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// Kill terminates the process group immediately.
func (p *Proc) Kill() {
	_ = p.stdin.Close()
	if p.cmd.Process != nil {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	}
	<-p.done
}

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nchungdev/agent-hub/internal/core"
)

// loginFlow drives one engine's CLI login (link / device code / pasted code)
// so the user can sign in from the GUI. The CLI stays the only credential store.
type loginFlow struct {
	id       string // identifies this attempt so a stale cancel cannot kill a newer one
	mu       sync.Mutex
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	out      strings.Builder
	running  bool
	finished bool
	failed   string
	started  time.Time
}

var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07`)
	urlRe  = regexp.MustCompile(`https://[^\s"'<>\x1b]+`)
	codeRe = regexp.MustCompile(`\b[A-Z0-9]{4}-[A-Z0-9]{4,5}\b`)
)

type loginState struct {
	Flow      string   `json:"flow"`
	Running   bool     `json:"running"`
	Finished  bool     `json:"finished"`
	Success   bool     `json:"success"`
	Error     string   `json:"error,omitempty"`
	URLs      []string `json:"urls"`
	Code      string   `json:"code,omitempty"`
	NeedsCode bool     `json:"needs_code"`
	Output    string   `json:"output"`
}

func (f *loginFlow) snapshot() loginState {
	f.mu.Lock()
	defer f.mu.Unlock()
	text := ansiRe.ReplaceAllString(f.out.String(), "")
	st := loginState{Flow: f.id, Running: f.running, Finished: f.finished, Success: f.finished && f.failed == "", Error: f.failed, Output: text, URLs: []string{}}
	seen := map[string]bool{}
	for _, u := range urlRe.FindAllString(text, -1) {
		u = strings.TrimRight(u, ".,)")
		if !seen[u] {
			seen[u] = true
			st.URLs = append(st.URLs, u)
		}
	}
	if m := codeRe.FindString(text); m != "" {
		st.Code = m
	}
	low := strings.ToLower(text)
	st.NeedsCode = f.running && (strings.Contains(low, "paste code") || strings.Contains(low, "paste the code"))
	return st
}

func (v *V2) loginFor(id string) (core.LoginProvider, bool) {
	for _, e := range v.Engines {
		if e.ID() == id {
			lp, ok := e.(core.LoginProvider)
			return lp, ok
		}
	}
	return nil, false
}

func (v *V2) startLogin(id string) (*loginFlow, error) {
	lp, ok := v.loginFor(id)
	if !ok {
		return nil, core.ErrNoSuchEngine
	}
	bin, args := lp.LoginCommand()
	if bin == "" {
		return nil, exec.ErrNotFound
	}
	v.loginMu.Lock()
	defer v.loginMu.Unlock()
	if v.logins == nil {
		v.logins = map[string]*loginFlow{}
	}
	if old := v.logins[id]; old != nil {
		old.cancel()
	}
	ctx, cancelCtx := context.WithTimeout(context.Background(), 15*time.Minute)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "TERM=dumb", "NO_COLOR=1", "BROWSER=true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancelCtx()
		return nil, err
	}
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	f := &loginFlow{id: uuid.NewString(), cmd: cmd, stdin: stdin, running: true, started: time.Now()}
	if err := cmd.Start(); err != nil {
		cancelCtx()
		return nil, err
	}
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		sc.Split(scanChunks)
		for sc.Scan() {
			f.mu.Lock()
			if f.out.Len() < 64*1024 {
				f.out.WriteString(sc.Text())
			}
			f.mu.Unlock()
		}
	}()
	go func() {
		err := cmd.Wait()
		_ = pw.Close()
		cancelCtx()
		// The CLI may have stored the credentials and still exit abnormally (e.g. it was stopped right
		// after the browser step finished); trust the engine's own login status over the exit code.
		signedIn := false
		if err != nil {
			for _, e := range v.Engines {
				if sp, ok := e.(core.StatusProvider); ok && e.ID() == id {
					st := sp.Status(context.Background())
					signedIn = st.Known && st.LoggedIn
				}
			}
		}
		f.mu.Lock()
		f.running, f.finished = false, true
		if err != nil && !signedIn {
			f.failed = "login did not complete: " + err.Error()
		}
		f.mu.Unlock()
		v.statusMu.Lock()
		delete(v.statusCache, id) // force a fresh auth check
		v.statusMu.Unlock()
	}()
	v.logins[id] = f
	return f, nil
}

// scanChunks yields whatever has arrived (prompts such as "Paste code here >" have no newline).
func scanChunks(data []byte, atEOF bool) (int, []byte, error) {
	if len(data) == 0 {
		return 0, nil, nil
	}
	return len(data), data, nil
}

func (f *loginFlow) cancel() {
	f.mu.Lock()
	running := f.running
	f.mu.Unlock()
	if running && f.cmd.Process != nil {
		_ = syscall.Kill(-f.cmd.Process.Pid, syscall.SIGTERM)
	}
}

func (v *V2) getLogin(id string) *loginFlow {
	v.loginMu.Lock()
	defer v.loginMu.Unlock()
	return v.logins[id]
}

func (v *V2) handleLoginStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Starting a CLI login while signed in can replace or clear the stored credentials, so the hub
	// refuses unless the caller explicitly asks to replace them (?replace=1).
	if r.URL.Query().Get("replace") != "1" {
		for _, e := range v.Engines {
			if sp, ok := e.(core.StatusProvider); ok && e.ID() == id {
				if st := sp.Status(r.Context()); st.Known && st.LoggedIn {
					httpError(w, errAlreadySignedIn, http.StatusConflict)
					return
				}
			}
		}
	}
	f, err := v.startLogin(id)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	time.Sleep(1500 * time.Millisecond) // let the CLI print its link/code
	jsonResponse(w, f.snapshot())
}

func (v *V2) handleLoginState(w http.ResponseWriter, r *http.Request) {
	f := v.getLogin(r.PathValue("id"))
	if f == nil {
		jsonResponse(w, loginState{URLs: []string{}})
		return
	}
	jsonResponse(w, f.snapshot())
}

func (v *V2) handleLoginInput(w http.ResponseWriter, r *http.Request) {
	f := v.getLogin(r.PathValue("id"))
	if f == nil {
		httpError(w, core.ErrNotLive, http.StatusNotFound)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		httpError(w, io.ErrUnexpectedEOF, http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	_, err := io.WriteString(f.stdin, strings.TrimSpace(req.Text)+"\n")
	f.mu.Unlock()
	if err != nil {
		httpError(w, err, http.StatusConflict)
		return
	}
	time.Sleep(1500 * time.Millisecond)
	jsonResponse(w, f.snapshot())
}

func (v *V2) handleLoginCancel(w http.ResponseWriter, r *http.Request) {
	if f := v.getLogin(r.PathValue("id")); f != nil {
		// ?flow=<id> restricts the cancel to that attempt (a late cancel of an old attempt is ignored)
		if want := r.URL.Query().Get("flow"); want == "" || want == f.id {
			f.cancel()
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func jsonDecode(s string, v any) error { return json.NewDecoder(strings.NewReader(s)).Decode(v) }

const errAlreadySignedIn = constErr("already signed in; starting a new sign-in may replace the stored credentials (pass replace=1 to do it anyway)")

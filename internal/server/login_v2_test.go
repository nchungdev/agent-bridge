package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nchungdev/agent-hub/internal/core"
	"github.com/nchungdev/agent-hub/internal/enginetest"
)

type loginEngine struct {
	*enginetest.Engine
	bin string
}

func (l loginEngine) LoginCommand() (string, []string) { return l.bin, nil }

func fakeLogin(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "fakelogin.sh")
	script := "#!/bin/sh\nprintf '\\033[94mOpen https://auth.example.com/device\\033[0m\\nEnter code ABCD-EF12\\nPaste code here > '\nread code\n[ \"$code\" = \"good\" ] && echo OK && exit 0\nexit 3\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func waitFinished(t *testing.T, v *V2, id string) loginState {
	for i := 0; i < 100; i++ {
		if st := v.getLogin(id).snapshot(); st.Finished {
			return st
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("login did not finish")
	return loginState{}
}

func TestLoginFlowSuccess(t *testing.T) {
	v := &V2{Engines: []core.Engine{loginEngine{enginetest.New("x"), fakeLogin(t)}}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v2/engines/x/login", nil)
	r.SetPathValue("id", "x")
	v.handleLoginStart(w, r)
	var st loginState
	if err := jsonDecode(w.Body.String(), &st); err != nil {
		t.Fatal(err)
	}
	if len(st.URLs) != 1 || st.URLs[0] != "https://auth.example.com/device" || st.Code != "ABCD-EF12" || !st.NeedsCode || !st.Running {
		t.Fatalf("state=%+v", st)
	}
	if strings.Contains(st.Output, "\x1b") {
		t.Fatal("ANSI not stripped")
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/api/v2/engines/x/login/input", strings.NewReader(`{"text":"good"}`))
	r.SetPathValue("id", "x")
	v.handleLoginInput(w, r)
	if fin := waitFinished(t, v, "x"); !fin.Success {
		t.Fatalf("state=%+v", fin)
	}
}

func TestLoginFlowFailureAndCancel(t *testing.T) {
	v := &V2{Engines: []core.Engine{loginEngine{enginetest.New("x"), fakeLogin(t)}}}
	if _, err := v.startLogin("x"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	f := v.getLogin("x")
	_, _ = f.stdin.Write([]byte("bad\n"))
	if fin := waitFinished(t, v, "x"); fin.Success || fin.Error == "" {
		t.Fatalf("state=%+v", fin)
	}
	f2, _ := v.startLogin("x")
	time.Sleep(300 * time.Millisecond)
	f2.cancel()
	if fin := waitFinished(t, v, "x"); fin.Success {
		t.Fatal("cancelled login must not succeed")
	}
	if _, err := v.startLogin("nope"); err == nil {
		t.Fatal("unknown engine must error")
	}
}

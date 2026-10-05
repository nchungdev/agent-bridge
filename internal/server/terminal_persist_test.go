package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func readUntil(t *testing.T, c *websocket.Conn, want string) string {
	t.Helper()
	var got bytes.Buffer
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for !strings.Contains(got.String(), want) {
		_, p, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for %q: %v (got %q)", want, err, got.String())
		}
		got.Write(p)
	}
	return got.String()
}

func TestTerminalReattachReplaysScreen(t *testing.T) {
	t.Run("in-process", func(t *testing.T) {
		t.Setenv("AGENT_BRIDGE_TMUX", "0")
		testReattach(t)
	})
	t.Run("tmux", func(t *testing.T) {
		if tmuxBin() == "" {
			t.Skip("tmux not installed")
		}
		isolateTmux(t)
		testReattach(t)
	})
}

func testReattach(t *testing.T) {
	home, _ := os.UserHomeDir()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/terminal", handleTerminalWS)
	mux.HandleFunc("GET /api/terminal/sessions", handleTerminalList)
	mux.HandleFunc("DELETE /api/terminal/sessions/{id}", handleTerminalKill)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/terminal?id=persisttest&dir=" + home

	c1, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = c1.WriteMessage(websocket.TextMessage, []byte("echo PERSIST_$((20+22))\n"))
	readUntil(t, c1, "PERSIST_42")
	c1.Close() // the page goes away; the shell must stay

	time.Sleep(200 * time.Millisecond)
	c2, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	readUntil(t, c2, "PERSIST_42") // replayed from scrollback

	resp0, err := http.Get(srv.URL + "/api/terminal/sessions")
	if err != nil {
		t.Fatal(err)
	}
	var list []struct{ ID string }
	_ = json.NewDecoder(resp0.Body).Decode(&list)
	resp0.Body.Close()
	if len(list) != 1 || list[0].ID != "persisttest" {
		t.Fatalf("session list = %+v", list)
	}

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/terminal/sessions/persisttest", nil)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 204 {
		t.Fatalf("kill failed: %v", err)
	}
	ptyMu.Lock()
	_, still := ptyRegistry["persisttest"]
	ptyMu.Unlock()
	if still {
		t.Fatal("session should be gone after DELETE")
	}
	if tmuxBin() != "" && os.Getenv("AGENT_BRIDGE_TMUX") != "0" {
		if _, err := tmuxRun("has-session", "-t", "=persisttest"); err == nil {
			t.Fatal("tmux session should be gone after DELETE")
		}
	}
}

func newTermServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/terminal", handleTerminalWS)
	mux.HandleFunc("DELETE /api/terminal/sessions/{id}", handleTerminalKill)
	return httptest.NewServer(mux)
}

// the point of tmux: a terminal and the program in it outlive the server process
func TestTmuxTerminalSurvivesServerRestart(t *testing.T) {
	if tmuxBin() == "" {
		t.Skip("tmux not installed")
	}
	isolateTmux(t)
	home, _ := os.UserHomeDir()
	srv1 := newTermServer()
	c1, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv1.URL, "http")+"/ws/terminal?id=survivetest&dir="+home, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = c1.WriteMessage(websocket.TextMessage, []byte("export KEEP=alive_$((6*7)); echo MARK$((1+1))\n"))
	readUntil(t, c1, "MARK2")
	c1.Close()
	srv1.Close() // "restart": the first server is gone entirely

	srv2 := newTermServer()
	defer srv2.Close()
	c2, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv2.URL, "http")+"/ws/terminal?id=survivetest&dir="+home, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	_ = c2.WriteMessage(websocket.TextMessage, []byte("echo $KEEP\n"))
	readUntil(t, c2, "alive_42") // same shell: its environment survived

	req, _ := http.NewRequest("DELETE", srv2.URL+"/api/terminal/sessions/survivetest", nil)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 204 {
		t.Fatalf("kill failed: %v", err)
	}
}

// isolateTmux points the server at a private tmux socket for the test, so it never lists or touches the
// terminals of a running Agent Bridge, and removes that tmux server afterwards.
func isolateTmux(t *testing.T) {
	t.Helper()
	t.Setenv("AGENT_BRIDGE_TMUX_SOCKET", "agent-bridge-test-"+strconv.Itoa(os.Getpid()))
	t.Cleanup(func() { _, _ = tmuxRun("kill-server") })
}

func TestTerminalBufferEndpoint(t *testing.T) {
	if tmuxBin() == "" {
		t.Skip("tmux not installed")
	}
	isolateTmux(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/terminal/sessions/{id}/buffer", handleTerminalBuffer)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	get := func() (int, string, string) {
		resp, err := http.Get(srv.URL + "/api/terminal/sessions/anything/buffer")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), resp.Header.Get("Content-Type")
	}

	if code, _, _ := get(); code != http.StatusNotFound {
		t.Fatalf("no tmux server yet: got %d, want 404", code)
	}
	home, _ := os.UserHomeDir()
	if err := ensureTmuxSession("buftest", home, "", "", []string{"bash"}, 80, 24); err != nil {
		t.Fatal(err)
	}
	if _, err := tmuxRun("set-buffer", "copied from the terminal"); err != nil {
		t.Fatal(err)
	}
	code, body, ctype := get()
	if code != 200 || body != "copied from the terminal" || ctype != "text/plain" {
		t.Fatalf("got %d %q %q", code, body, ctype)
	}
}

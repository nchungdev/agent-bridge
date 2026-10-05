package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
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
}

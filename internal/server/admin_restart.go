package server

import (
	"log"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"
)

// internalRemote: loopback or a private LAN/Docker-bridge address. The restart endpoint is meant for
// ClaraOS on the same host or network, never for the open internet.
func internalRemote(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

// handleAdminRestart re-executes the server binary in place (same PID, args and environment), so a
// rebuilt binary is picked up and no process supervisor is required. Every running terminal and
// agent CLI session ends with the process.
func handleAdminRestart(w http.ResponseWriter, r *http.Request) {
	if !internalRemote(r) || !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	log.Printf("[admin] restart requested by %s", r.RemoteAddr)
	go func() {
		time.Sleep(300 * time.Millisecond) // let the response reach the caller
		if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
			log.Printf("[admin] re-exec failed: %v", err)
		}
	}()
}

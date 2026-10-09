package server

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/nchungdev/agent-bridge/internal/agent"
	"github.com/nchungdev/agent-bridge/internal/bridge"
	"github.com/nchungdev/agent-bridge/internal/config"
	"github.com/nchungdev/agent-bridge/internal/session"
)

type Server struct {
	cfg        *config.Config
	hub        *Hub
	sm         *session.Manager
	dispatcher *agent.Dispatcher
	webFS      fs.FS
	v2         *V2
	bm         *bridge.Manager
	db         *sql.DB
	zalo       http.Handler
}

func New(cfg *config.Config, sm *session.Manager, dispatcher *agent.Dispatcher, webFS fs.FS, database *sql.DB) *Server {
	return &Server{
		cfg:        cfg,
		hub:        NewHub(),
		sm:         sm,
		dispatcher: dispatcher,
		webFS:      webFS,
		db:         database,
		bm:         bridge.NewManager(database),
	}
}

// EnableV2 turns on the engine-agnostic /ws/v2 transport (AGENT_BRIDGE_V2=1).
func (s *Server) EnableV2(v *V2) { s.v2 = v }

// EnableZalo serves h at zaloWebhookPath. The route is authenticated by Zalo's own secret header instead of
// AGENT_BRIDGE_TOKEN, because Zalo cannot send that token.
func (s *Server) EnableZalo(h http.Handler) { s.zalo = h }

const zaloWebhookPath = "/api/zalo/webhook"

func (s *Server) Start() error {
	// Start WebSocket hub
	go s.hub.Run()

	mux := http.NewServeMux()

	// Register API routes
	Routes(mux, s.hub, s.sm, s.dispatcher)
	if s.bm != nil {
		RegisterBridgeRoutes(mux, s.bm, s.db)
	}
	if s.v2 != nil {
		s.v2.Routes(mux)
	}

	// Serve embedded frontend
	fileServer := http.FileServer(http.FS(s.webFS))

	// SPA fallback: serve static files if they exist, otherwise fallback to index.html
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Remove leading slash for fs.Stat
		cleanPath := path
		if len(cleanPath) > 0 && cleanPath[0] == '/' {
			cleanPath = cleanPath[1:]
		}

		// Check if file exists in webFS
		if _, err := fs.Stat(s.webFS, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback to index.html for client-side routing
		indexBytes, err := fs.ReadFile(s.webFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexBytes)
	})

	host := os.Getenv("AGENT_BRIDGE_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	// the terminal is a remote shell: never expose it beyond loopback without a token (or explicit AGENT_BRIDGE_ALLOW_LAN=1)
	if !isLoopbackHost(host) && os.Getenv("AGENT_BRIDGE_TOKEN") == "" && os.Getenv("AGENT_BRIDGE_ALLOW_LAN") != "1" {
		return fmt.Errorf("AGENT_BRIDGE_HOST=%s is reachable from the network: set AGENT_BRIDGE_TOKEN (or bind to 127.0.0.1)", host)
	}
	addr := net.JoinHostPort(host, strconv.Itoa(s.cfg.Port))
	log.Printf("🚀 Agent Bridge running at http://%s", addr)
	guarded := authMiddleware(mux)
	handler := http.Handler(guarded)
	if s.zalo != nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == zaloWebhookPath {
				s.zalo.ServeHTTP(w, r)
				return
			}
			guarded.ServeHTTP(w, r)
		})
	}
	return http.ListenAndServe(addr, handler)
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

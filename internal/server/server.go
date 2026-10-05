package server

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/nchungdev/agent-hub/internal/agent"
	"github.com/nchungdev/agent-hub/internal/bridge"
	"github.com/nchungdev/agent-hub/internal/config"
	"github.com/nchungdev/agent-hub/internal/session"
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

// EnableV2 turns on the engine-agnostic /ws/v2 transport (AGENT_HUB_V2=1).
func (s *Server) EnableV2(v *V2) { s.v2 = v }

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

	addr := fmt.Sprintf("127.0.0.1:%d", s.cfg.Port)
	log.Printf("🚀 Agent Hub running at http://%s", addr)
	return http.ListenAndServe(addr, authMiddleware(mux))
}

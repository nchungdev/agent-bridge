package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/nchungdev/agent-hub/internal/adapters/agy"
	"github.com/nchungdev/agent-hub/internal/adapters/claude"
	"github.com/nchungdev/agent-hub/internal/adapters/codex"
	"github.com/nchungdev/agent-hub/internal/agent"
	"github.com/nchungdev/agent-hub/internal/config"
	"github.com/nchungdev/agent-hub/internal/core"
	"github.com/nchungdev/agent-hub/internal/db"
	"github.com/nchungdev/agent-hub/internal/manager"
	"github.com/nchungdev/agent-hub/internal/server"
	"github.com/nchungdev/agent-hub/internal/session"
	"github.com/nchungdev/agent-hub/internal/store"
)

//go:embed all:web/dist
var webFiles embed.FS

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	// Load config
	cfg := config.Load()
	log.Printf("📁 Data directory: %s", cfg.DataDir)

	// Open database
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("❌ Database error: %v", err)
	}
	defer database.Close()
	log.Println("✅ Database initialized")

	// Initialize session manager
	sm := session.NewManager(database)

	// Initialize agent dispatcher
	dispatcher := agent.NewDispatcher(cfg)
	agentList := dispatcher.ListAgents()
	for _, a := range agentList {
		status := "❌ not found"
		if a.Available {
			status = "✅ available"
		}
		log.Printf("🤖 Agent: %s (%s) %s", a.ID, a.DisplayName, status)
	}

	// Extract embedded frontend FS
	webFS, err := fs.Sub(webFiles, "web/dist")
	if err != nil {
		log.Fatalf("❌ Failed to load embedded frontend: %v", err)
	}

	// Start HTTP server
	srv := server.New(cfg, sm, dispatcher, webFS)
	if os.Getenv("AGENT_HUB_V2") == "1" {
		st, err := store.New(database)
		if err != nil {
			log.Fatalf("❌ v2 store: %v", err)
		}
		engines := []core.Engine{
			claude.New(cfg.Agents["claude"].BinaryPath),
			codex.New(cfg.Agents["codex"].BinaryPath),
			agy.New(""),
		}
		maxLive, _ := strconv.Atoi(os.Getenv("AGENT_HUB_MAX_LIVE"))
		mgr := manager.New(st, engines, manager.Config{MaxLive: maxLive, IdleTimeout: 20 * time.Minute})
		if n, err := mgr.Recover(); err == nil && n > 0 {
			log.Printf("♻️  v2: %d binding(s) marked suspended after restart", n)
		}
		go mgr.Run(context.Background())
		home, _ := os.UserHomeDir()
		v2 := &server.V2{Mgr: mgr, Store: st, Engines: engines, Convs: sm, DefaultWorkspace: home}
		go v2.Warm()
		srv.EnableV2(v2)
		log.Println("🧪 Agent Hub v2 transport enabled (/ws/v2, /api/v2/*)")
	}
	log.Fatal(srv.Start())
}

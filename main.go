package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nchungdev/agent-bridge/internal/accounts"
	"github.com/nchungdev/agent-bridge/internal/adapters/agy"
	"github.com/nchungdev/agent-bridge/internal/adapters/claude"
	"github.com/nchungdev/agent-bridge/internal/adapters/codex"
	"github.com/nchungdev/agent-bridge/internal/agent"
	"github.com/nchungdev/agent-bridge/internal/config"
	"github.com/nchungdev/agent-bridge/internal/core"
	"github.com/nchungdev/agent-bridge/internal/db"
	"github.com/nchungdev/agent-bridge/internal/manager"
	"github.com/nchungdev/agent-bridge/internal/server"
	"github.com/nchungdev/agent-bridge/internal/session"
	"github.com/nchungdev/agent-bridge/internal/store"
)

//go:embed all:web/dist
var webFiles embed.FS

func main() {
	if runCLI(os.Args[1:]) { // version, update, service ...: do the job and exit instead of starting the server
		return
	}
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
	srv := server.New(cfg, sm, dispatcher, webFS, database)
	if os.Getenv("AGENT_BRIDGE_V2") != "0" { // on by default; 0 turns it off
		st, err := store.New(database)
		if err != nil {
			log.Fatalf("❌ v2 store: %v", err)
		}
		engines := []core.Engine{
			claude.New(cfg.Agents["claude"].BinaryPath),
			codex.New(cfg.Agents["codex"].BinaryPath),
			agy.New(""),
		}
		if n, err := st.RepairEvents(); err != nil {
			log.Printf("⚠️  v2 repair: %v", err)
		} else if n > 0 {
			log.Printf("🩹 v2: repaired %d damaged event row(s)", n)
		}
		home, _ := os.UserHomeDir()
		registry, err := accounts.New(st, filepath.Join(home, ".agent-bridge", "profiles"), engines)
		if err != nil {
			log.Fatalf("❌ accounts: %v", err)
		}
		allEngines := registry.Engines() // base engines + any saved extra accounts
		maxLive, _ := strconv.Atoi(os.Getenv("AGENT_BRIDGE_MAX_LIVE"))
		every := 5
		if v := os.Getenv("AGENT_BRIDGE_SUMMARY_EVERY"); v != "" {
			every, _ = strconv.Atoi(v) // 0 disables rolling summaries
		}
		// Summarizers, cheapest-first by typical price of each engine's smallest model; unusable ones
		// (signed out / failing) are skipped at run time, so any single working engine is enough.
		var summarizers []core.Summarizer
		for _, id := range []string{"claude", "agy", "codex"} {
			for _, e := range engines {
				if sm, ok := e.(core.Summarizer); ok && e.ID() == id {
					summarizers = append(summarizers, sm)
				}
			}
		}
		mgr := manager.New(st, allEngines, manager.Config{MaxLive: maxLive, IdleTimeout: 20 * time.Minute, Summarizers: summarizers, SummaryEvery: every})
		if n, err := mgr.Recover(); err == nil && n > 0 {
			log.Printf("♻️  v2: %d binding(s) marked suspended after restart", n)
		}
		go mgr.Run(context.Background())
		registry.OnAdd = mgr.RegisterEngine
		registry.OnRemove = mgr.UnregisterEngine
		v2 := &server.V2{Mgr: mgr, Store: st, Engines: engines, Registry: registry, Convs: sm, DefaultWorkspace: home, DataDir: cfg.DataDir}
		go v2.Warm()
		srv.EnableV2(v2)
		log.Println("🧪 Agent Bridge v2 transport enabled (/ws/v2, /api/v2/*)")
	}
	log.Fatal(srv.Start())
}

package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/nchungdev/agent-bridge/internal/manager"
	"github.com/nchungdev/agent-bridge/internal/nas"
	"github.com/nchungdev/agent-bridge/internal/server"
	"github.com/nchungdev/agent-bridge/internal/session"
	"github.com/nchungdev/agent-bridge/internal/store"
	"github.com/nchungdev/agent-bridge/internal/zalo"
)

// enableZalo turns on the Zalo bot when it is configured: it adapts the application services to the
// interfaces the zalo package needs and registers the webhook on the server.
func enableZalo(srv *server.Server, mgr *manager.Manager, st *store.Store, sm *session.Manager, v2 *server.V2, dataDir, home string) {
	cfg, ok := zalo.LoadConfig()
	if !ok {
		if os.Getenv("ZALO_BOT_TOKEN") != "" {
			log.Println("💬 Zalo bot NOT enabled: set ZALO_WEBHOOK_SECRET (8-256 chars) and ZALO_ALLOWED_IDS")
		}
		return
	}
	if cfg.Workspace == "" {
		cfg.Workspace = home
	}
	svc := zalo.New(cfg, zalo.Deps{
		Agent:     mgr,
		Settings:  st,
		Messenger: zalo.NewClient(cfg),
		Images:    zalo.NewImageStore(filepath.Join(dataDir, "uploads")),
		Host:      hostAdapter{nas.New(nas.ExecRunner{}, cfg.NASUnits)},
		PathOK:    server.PathAllowed,
		NewConv: func(name, ws string) (string, error) {
			s, err := sm.CreateSession(name, ws)
			if err != nil {
				return "", err
			}
			return s.ID, nil
		},
		Convs: func() ([]zalo.ConvInfo, error) { return listConvs(sm, st) },
		Engines: func() []zalo.EngineInfo {
			var out []zalo.EngineInfo
			for _, e := range v2.ListEngines() {
				out = append(out, zalo.EngineInfo{ID: e.ID(), Modes: e.Capabilities().PermissionModes})
			}
			return out
		},
	})
	svc.Start(context.Background())
	srv.EnableZalo(svc.Handler())
	log.Printf("💬 Zalo bot enabled (%s, engine %s, mode %s, %d allowed user(s))", zalo.WebhookPath, cfg.Engine, cfg.Mode, len(cfg.Allowed))
}

// listConvs reads the conversations the web UI shows, with their titles and archived flag.
func listConvs(sm *session.Manager, st *store.Store) ([]zalo.ConvInfo, error) {
	sessions, err := sm.ListSessions()
	if err != nil {
		return nil, err
	}
	meta, _ := st.ListMeta()
	out := make([]zalo.ConvInfo, 0, len(sessions))
	for _, s := range sessions {
		m := meta[s.ID]
		title := m.Title
		if title == "" {
			title = s.Name
		}
		out = append(out, zalo.ConvInfo{ID: s.ID, Title: title, Workspace: s.Workspace, Updated: s.UpdatedAt, Archived: m.Archived})
	}
	return out, nil
}

// hostAdapter lets nas.Host serve as the zalo.Host port: only the service type differs.
type hostAdapter struct{ *nas.Host }

func (a hostAdapter) Services(ctx context.Context) ([]zalo.HostService, error) {
	list, err := a.Host.Services(ctx)
	out := make([]zalo.HostService, len(list))
	for i, s := range list {
		out[i] = zalo.HostService{Name: s.Name, Description: s.Description}
	}
	return out, err
}

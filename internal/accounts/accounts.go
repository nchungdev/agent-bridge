// Package accounts lets one engine have several logins. Each extra account owns a private config dir
// (CLAUDE_CONFIG_DIR / CODEX_HOME), and is exposed to the rest of the hub as a virtual engine whose id is
// "<engine>@<slug>" (the engine's own id is the default account). Everything keyed by engine id —
// sessions, quota, sign-in, commands — therefore becomes per-account without further changes.
package accounts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/nchungdev/agent-bridge/internal/core"
	"github.com/nchungdev/agent-bridge/internal/store"
)

var (
	ErrUnsupported = errors.New("this engine cannot keep several accounts")
	ErrNotFound    = errors.New("no such account")
)

// shared lists what a new account inherits from the default config dir (settings, skills, plugins…) by
// symlink; credentials, history and sessions are never shared.
var shared = map[string][]string{
	"claude": {"settings.json", "CLAUDE.md", "skills", "agents", "commands", "plugins", "output-styles", "keybindings.json"},
	"codex":  {"config.toml", "skills", "prompts", "AGENTS.md"},
}

func defaultDir(engine string) string {
	home, _ := os.UserHomeDir()
	switch engine {
	case "claude":
		if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
			return d
		}
		return filepath.Join(home, ".claude")
	case "codex":
		if d := os.Getenv("CODEX_HOME"); d != "" {
			return d
		}
		return filepath.Join(home, ".codex")
	}
	return ""
}

// Account is one login of an engine. The default account has Dir == "".
type Account struct {
	ID      string `json:"id"`
	Engine  string `json:"engine"`
	Label   string `json:"label"`
	Dir     string `json:"-"`
	Default bool   `json:"default"`
}

type Registry struct {
	st   *store.Store
	root string

	mu       sync.RWMutex
	base     []core.Engine
	accts    map[string]*Account
	order    []string
	virt     map[string]*accountEngine
	OnAdd    func(core.Engine)
	OnRemove func(id string)
}

// New builds the registry and re-creates the virtual engines saved in the store.
func New(st *store.Store, root string, base []core.Engine) (*Registry, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	r := &Registry{st: st, root: root, base: base, accts: map[string]*Account{}, virt: map[string]*accountEngine{}}
	for _, e := range base {
		if _, ok := e.(core.ProfileCapable); ok {
			r.accts[e.ID()] = &Account{ID: e.ID(), Engine: e.ID(), Label: "Default", Default: true}
			r.order = append(r.order, e.ID())
		}
	}
	rows, err := st.ListAccounts()
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if be := r.baseEngine(row.Engine); be != nil {
			r.add(&Account{ID: row.ID, Engine: row.Engine, Label: row.Label, Dir: row.Dir}, be)
		}
	}
	return r, nil
}

func (r *Registry) baseEngine(id string) core.Engine {
	for _, e := range r.base {
		if e.ID() == id {
			return e
		}
	}
	return nil
}

func (r *Registry) add(a *Account, base core.Engine) *accountEngine {
	ae := &accountEngine{Engine: base, id: a.ID, dir: a.Dir, label: a.Label}
	r.accts[a.ID] = a
	r.order = append(r.order, a.ID)
	r.virt[a.ID] = ae
	return ae
}

// Engines returns the base engines followed by every extra account.
func (r *Registry) Engines() []core.Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := append([]core.Engine(nil), r.base...)
	for _, id := range r.order {
		if v, ok := r.virt[id]; ok {
			out = append(out, v)
		}
	}
	return out
}

// List returns all accounts, defaults first per engine.
func (r *Registry) List() []Account {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Account
	for _, id := range r.order {
		out = append(out, *r.accts[id])
	}
	return out
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Create makes a new, signed-out account for an engine.
func (r *Registry) Create(engine, label string) (*Account, core.Engine, error) {
	base := r.baseEngine(engine)
	if base == nil {
		return nil, nil, core.ErrNoSuchEngine
	}
	if _, ok := base.(core.ProfileCapable); !ok {
		return nil, nil, ErrUnsupported
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, nil, fmt.Errorf("a label is required")
	}
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(label), "-"), "-")
	if slug == "" {
		slug = "account"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := engine + "@" + slug
	for i := 2; r.accts[id] != nil; i++ {
		id = fmt.Sprintf("%s@%s-%d", engine, slug, i)
	}
	dir := filepath.Join(r.root, engine, strings.TrimPrefix(id, engine+"@"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	linkShared(engine, defaultDir(engine), dir)
	a := &Account{ID: id, Engine: engine, Label: label, Dir: dir}
	if err := r.st.AddAccount(store.AccountRow{ID: id, Engine: engine, Label: label, Dir: dir}); err != nil {
		return nil, nil, err
	}
	ae := r.add(a, base)
	if r.OnAdd != nil {
		r.OnAdd(ae)
	}
	return a, ae, nil
}

// Rename changes the display label.
func (r *Registry) Rename(id, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return fmt.Errorf("a label is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.accts[id]
	if a == nil || a.Default {
		return ErrNotFound
	}
	if err := r.st.RenameAccount(id, label); err != nil {
		return err
	}
	a.Label = label
	if v := r.virt[id]; v != nil {
		v.label = label
	}
	return nil
}

// Remove deletes an extra account and its private config dir (never the default account).
func (r *Registry) Remove(id string) error {
	r.mu.Lock()
	a := r.accts[id]
	if a == nil || a.Default {
		r.mu.Unlock()
		return ErrNotFound
	}
	delete(r.accts, id)
	delete(r.virt, id)
	for i, x := range r.order {
		if x == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	r.mu.Unlock()
	if r.OnRemove != nil {
		r.OnRemove(id)
	}
	if err := r.st.DeleteAccount(id); err != nil {
		return err
	}
	// only ever delete inside our own profiles root
	if rel, err := filepath.Rel(r.root, a.Dir); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		_ = os.RemoveAll(a.Dir)
	}
	if r.Active(a.Engine) == id {
		_ = r.SetActive(a.Engine, a.Engine)
	}
	return nil
}

// Active is the account new conversations use for an engine (the engine id itself = default account).
func (r *Registry) Active(engine string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if v := r.st.GetSetting("active_account:" + engine); v != "" && r.accts[v] != nil {
		return v
	}
	return engine
}

func (r *Registry) SetActive(engine, id string) error {
	r.mu.RLock()
	a := r.accts[id]
	r.mu.RUnlock()
	if a == nil || a.Engine != engine {
		return ErrNotFound
	}
	return r.st.SetSetting("active_account:"+engine, id)
}

func linkShared(engine, from, to string) {
	for _, name := range shared[engine] {
		src := filepath.Join(from, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		_ = os.Symlink(src, filepath.Join(to, name))
	}
}

// accountEngine is the virtual engine for an extra account: the base engine plus a config dir.
type accountEngine struct {
	core.Engine // Capabilities, Models (overridden below)
	id          string
	dir         string
	label       string
}

func (a *accountEngine) ID() string                            { return a.id }
func (a *accountEngine) Base() string                          { return a.Engine.ID() }
func (a *accountEngine) Label() string                         { return a.label }
func (a *accountEngine) Dir() string                           { return a.dir }
func (a *accountEngine) ctx(c context.Context) context.Context { return core.WithProfileDir(c, a.dir) }

func (a *accountEngine) Models(ctx context.Context) ([]core.Model, error) {
	return a.Engine.Models(a.ctx(ctx))
}

func (a *accountEngine) Start(ctx context.Context, o core.StartOpts) (core.Session, error) {
	o.ProfileDir = a.dir
	return a.Engine.Start(ctx, o)
}

func (a *accountEngine) Status(ctx context.Context) core.AuthStatus {
	if sp, ok := a.Engine.(core.StatusProvider); ok {
		return sp.Status(a.ctx(ctx))
	}
	return core.AuthStatus{Installed: true}
}

func (a *accountEngine) Quota(ctx context.Context) (core.Quota, error) {
	qp, ok := a.Engine.(core.QuotaProvider)
	if !ok {
		return core.Quota{Engine: a.id}, fmt.Errorf("no quota source")
	}
	q, err := qp.Quota(a.ctx(ctx))
	q.Engine = a.id
	return q, err
}

func (a *accountEngine) Commands(ctx context.Context) ([]core.Command, error) {
	cl, ok := a.Engine.(core.CommandLister)
	if !ok {
		return nil, nil
	}
	return cl.Commands(a.ctx(ctx))
}

func (a *accountEngine) LoginCommand() (string, []string) {
	if lp, ok := a.Engine.(core.LoginProvider); ok {
		return lp.LoginCommand()
	}
	return "", nil
}

func (a *accountEngine) LoginEnv() []string {
	if pc, ok := a.Engine.(core.ProfileCapable); ok {
		return pc.ProfileEnv(a.dir)
	}
	return nil
}

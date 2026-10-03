package accounts_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nchungdev/agent-hub/internal/accounts"
	"github.com/nchungdev/agent-hub/internal/core"
	"github.com/nchungdev/agent-hub/internal/enginetest"
	"github.com/nchungdev/agent-hub/internal/store"
	_ "modernc.org/sqlite"
)

// profEngine is a fake engine that supports account dirs and reports which dir it was asked about.
type profEngine struct {
	*enginetest.Engine
	started []string
}

func (p *profEngine) ProfileEnv(dir string) []string { return []string{"FAKE_HOME=" + dir} }
func (p *profEngine) Start(ctx context.Context, o core.StartOpts) (core.Session, error) {
	p.started = append(p.started, o.ProfileDir)
	return p.Engine.Start(ctx, o)
}
func (p *profEngine) Status(ctx context.Context) core.AuthStatus {
	d := core.ProfileDirFrom(ctx)
	return core.AuthStatus{Installed: true, Known: true, LoggedIn: d == "", Detail: "dir=" + d}
}

func newReg(t *testing.T) (*accounts.Registry, *profEngine, *store.Store, string) {
	db, err := sql.Open("sqlite", "file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	st, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	base := &profEngine{Engine: enginetest.New("claude")}
	root := t.TempDir()
	reg, err := accounts.New(st, root, []core.Engine{base})
	if err != nil {
		t.Fatal(err)
	}
	return reg, base, st, root
}

func TestCreateAccountIsAnIsolatedVirtualEngine(t *testing.T) {
	reg, base, _, root := newReg(t)
	var added []string
	reg.OnAdd = func(e core.Engine) { added = append(added, e.ID()) }
	a, eng, err := reg.Create("claude", "Work Account")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "claude@work-account" || eng.ID() != a.ID || len(added) != 1 || added[0] != a.ID {
		t.Fatalf("a=%+v eng=%s added=%v", a, eng.ID(), added)
	}
	if rel, _ := filepath.Rel(root, a.Dir); strings.HasPrefix(rel, "..") {
		t.Fatalf("account dir escapes the profiles root: %s", a.Dir)
	}
	// sessions started through the account carry its config dir; the default account does not
	if _, err := eng.Start(context.Background(), core.StartOpts{}); err != nil {
		t.Fatal(err)
	}
	if len(base.started) != 1 || base.started[0] != a.Dir {
		t.Fatalf("started=%v want dir %s", base.started, a.Dir)
	}
	// status is evaluated per account
	if st := eng.(core.StatusProvider).Status(context.Background()); st.LoggedIn || st.Detail != "dir="+a.Dir {
		t.Fatalf("new account must start signed out, got %+v", st)
	}
	if st := base.Status(context.Background()); !st.LoggedIn {
		t.Fatalf("default account must be untouched, got %+v", st)
	}
	if env := eng.(core.LoginEnvProvider).LoginEnv(); len(env) != 1 || env[0] != "FAKE_HOME="+a.Dir {
		t.Fatalf("login env=%v", env)
	}
	if got := reg.Engines(); len(got) != 2 || got[1].ID() != a.ID {
		t.Fatalf("engines=%v", got)
	}
}

func TestIdsAreUniqueAndUnsupportedEnginesRefused(t *testing.T) {
	reg, _, _, _ := newReg(t)
	a1, _, _ := reg.Create("claude", "Work")
	a2, _, err := reg.Create("claude", "work")
	if err != nil || a1.ID == a2.ID {
		t.Fatalf("ids %s %s err=%v", a1.ID, a2.ID, err)
	}
	if _, _, err := reg.Create("nope", "x"); err == nil {
		t.Fatal("unknown engine must be refused")
	}
	if _, _, err := reg.Create("claude", "  "); err == nil {
		t.Fatal("empty label must be refused")
	}
}

func TestSharedSettingsLinkedButNotCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	def := filepath.Join(home, ".claude")
	_ = os.MkdirAll(filepath.Join(def, "skills"), 0o755)
	_ = os.WriteFile(filepath.Join(def, "settings.json"), []byte("{}"), 0o600)
	_ = os.WriteFile(filepath.Join(def, ".credentials.json"), []byte("secret"), 0o600)
	_ = os.MkdirAll(filepath.Join(def, "projects"), 0o755)
	reg, _, _, _ := newReg(t)
	a, _, err := reg.Create("claude", "Work")
	if err != nil {
		t.Fatal(err)
	}
	for _, shared := range []string{"settings.json", "skills"} {
		if fi, err := os.Lstat(filepath.Join(a.Dir, shared)); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s should be a symlink: %v", shared, err)
		}
	}
	for _, private := range []string{".credentials.json", "projects"} {
		if _, err := os.Lstat(filepath.Join(a.Dir, private)); err == nil {
			t.Fatalf("%s must NOT be shared with another account", private)
		}
	}
}

func TestRemovePersistAndActive(t *testing.T) {
	reg, _, st, root := newReg(t)
	a, _, _ := reg.Create("claude", "Work")
	if reg.Active("claude") != "claude" {
		t.Fatal("default account is active initially")
	}
	if err := reg.SetActive("claude", a.ID); err != nil || reg.Active("claude") != a.ID {
		t.Fatalf("set active failed: %v", err)
	}
	if err := reg.SetActive("claude", "claude@ghost"); err == nil {
		t.Fatal("unknown account must be refused")
	}
	if err := reg.Rename(a.ID, "Team"); err != nil {
		t.Fatal(err)
	}
	// a fresh registry (restart) restores accounts and labels from the store
	reg2, err := accounts.New(st, root, reg.Engines()[:1])
	if err != nil {
		t.Fatal(err)
	}
	var found *accounts.Account
	for _, x := range reg2.List() {
		if x.ID == a.ID {
			x := x
			found = &x
		}
	}
	if found == nil || found.Label != "Team" || reg2.Active("claude") != a.ID {
		t.Fatalf("not restored: %+v active=%s", found, reg2.Active("claude"))
	}
	if err := reg2.Remove("claude"); err == nil {
		t.Fatal("the default account cannot be removed")
	}
	if err := reg2.Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Dir); err == nil {
		t.Fatal("account dir should be deleted")
	}
	if reg2.Active("claude") != "claude" {
		t.Fatal("removing the active account falls back to default")
	}
}

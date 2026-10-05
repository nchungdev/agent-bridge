package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// AGYProfiles uses AGY Manager's credential snapshots. Switching is machine-wide,
// not an isolated virtual engine: all AGY sessions must be restarted afterwards.
type AGYProfiles struct {
	Home    string
	Restart func(context.Context) error
	mu      sync.Mutex
}
type AGYProfile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}
type AGYProfileList struct {
	Active   string       `json:"active_profile"`
	Profiles []AGYProfile `json:"profiles"`
}

func (a *AGYProfiles) home() string {
	if a.Home != "" {
		return a.Home
	}
	h, _ := os.UserHomeDir()
	return h
}
func (a *AGYProfiles) path() string {
	return filepath.Join(a.home(), ".gemini", "profiles", "profiles.json")
}
func (a *AGYProfiles) List() (AGYProfileList, error) {
	b, err := os.ReadFile(a.path())
	if os.IsNotExist(err) {
		return AGYProfileList{}, nil
	}
	var p AGYProfileList
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(b, &p)
	return p, err
}
func (a *AGYProfiles) Switch(ctx context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
		return ErrNotFound
	}
	p, err := a.List()
	if err != nil {
		return err
	}
	found := false
	for _, x := range p.Profiles {
		if x.ID == id {
			found = true
		}
	}
	if !found {
		return ErrNotFound
	}
	if p.Active == id {
		return nil
	}
	root := filepath.Join(a.home(), ".gemini")
	src := filepath.Join(root, "profiles", id)
	// Read and validate every source before changing any live credential.
	names := []string{"antigravity-oauth-token", "oauth_creds.json", "google_accounts.json"}
	paths := []string{filepath.Join(root, "antigravity-cli", names[0]), filepath.Join(root, names[1]), filepath.Join(root, names[2])}
	data := make([][]byte, len(names))
	for i, name := range names {
		data[i], err = os.ReadFile(filepath.Join(src, name))
		if os.IsNotExist(err) && i > 0 {
			data[i] = nil
			continue
		}
		if err != nil {
			return fmt.Errorf("read profile %s: %w", name, err)
		}
		if !json.Valid(data[i]) {
			return fmt.Errorf("invalid profile %s", name)
		}
	}
	old := make([][]byte, len(paths))
	for i, path := range paths {
		old[i], err = os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	metadata, err := os.ReadFile(a.path())
	if err != nil {
		return err
	}
	rollback := func() {
		for i, path := range paths {
			if old[i] == nil {
				_ = os.Remove(path)
			} else {
				_ = atomicPrivate(path, old[i])
			}
		}
		_ = atomicPrivate(a.path(), metadata)
	}
	for i, path := range paths {
		// Missing optional credentials must not retain a different user's login.
		if data[i] == nil {
			err = os.Remove(path)
			if os.IsNotExist(err) {
				err = nil
			}
		} else {
			err = atomicPrivate(path, data[i])
		}
		if err != nil {
			rollback()
			return err
		}
	}
	var document map[string]json.RawMessage
	if err = json.Unmarshal(metadata, &document); err != nil {
		rollback()
		return err
	}
	document["active_profile"], _ = json.Marshal(id)
	b, err := json.MarshalIndent(document, "", "  ")
	if err == nil {
		err = atomicPrivate(a.path(), b)
	}
	if err != nil {
		rollback()
		return err
	}
	restart := a.Restart
	if restart == nil {
		restart = func(ctx context.Context) error {
			// agent-hub runs as a system service: it has no user-session bus, so `systemctl --user` needs these.
			runtime := os.Getenv("XDG_RUNTIME_DIR")
			if runtime == "" {
				runtime = fmt.Sprintf("/run/user/%d", os.Getuid())
			}
			cmd := exec.CommandContext(ctx, "systemctl", "--user", "restart", "antigravity-cli-daemon.service")
			cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+runtime, "DBUS_SESSION_BUS_ADDRESS=unix:path="+runtime+"/bus")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		}
	}
	if err := restart(ctx); err != nil {
		rollback()
		_ = restart(ctx)
		return fmt.Errorf("restart AGY daemon: %w", err)
	}
	return nil
}
func atomicPrivate(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".agy-profile-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

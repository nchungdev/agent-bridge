package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nchungdev/agent-bridge/internal/accounts"
	"github.com/nchungdev/agent-bridge/internal/core"
)

type accountOut struct {
	accounts.Account
	Active   bool   `json:"active"`
	Known    bool   `json:"known"`
	LoggedIn bool   `json:"logged_in"`
	Detail   string `json:"detail,omitempty"`
	CanLogin bool   `json:"can_login"`
}

type accountGroup struct {
	Engine   string       `json:"engine"`
	Active   string       `json:"active"`
	Accounts []accountOut `json:"accounts"`
}

// handleAccounts lists the accounts of every engine that supports several logins, with sign-in status.
func (v *V2) handleAccounts(w http.ResponseWriter, r *http.Request) {
	out := []accountGroup{}
	if v.Registry == nil {
		jsonResponse(w, out)
		return
	}
	force := r.URL.Query().Get("refresh") == "1"
	byID := map[string]core.Engine{}
	for _, e := range v.engines() {
		byID[e.ID()] = e
	}
	idx := map[string]int{}
	for _, a := range v.Registry.List() {
		gi, ok := idx[a.Engine]
		if !ok {
			out = append(out, accountGroup{Engine: a.Engine, Active: v.Registry.Active(a.Engine), Accounts: []accountOut{}})
			gi = len(out) - 1
			idx[a.Engine] = gi
		}
		ao := accountOut{Account: a, Active: out[gi].Active == a.ID}
		if e := byID[a.ID]; e != nil {
			if sp, ok := e.(core.StatusProvider); ok {
				st := v.cachedStatus(r.Context(), a.ID, sp, force)
				ao.Known, ao.LoggedIn, ao.Detail = st.Known, st.LoggedIn, st.Detail
			}
			_, ao.CanLogin = e.(core.LoginProvider)
		}
		out[gi].Accounts = append(out[gi].Accounts, ao)
	}
	for _, e := range v.engines() {
		if e.ID() != "agy" {
			continue
		}
		profiles, err := v.agyProfiles().List()
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		g := accountGroup{Engine: "agy", Active: "agy@" + profiles.Active, Accounts: []accountOut{}}
		for _, p := range profiles.Profiles {
			g.Accounts = append(g.Accounts, accountOut{Account: accounts.Account{ID: "agy@" + p.ID, Engine: "agy", Label: p.Name}, Active: profiles.Active == p.ID, Known: false, Detail: p.Email})
		}
		out = append(out, g)
	}
	jsonResponse(w, out)
}

func (v *V2) handleAccountCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Engine string `json:"engine"`
		Label  string `json:"label"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil || v.Registry == nil {
		httpError(w, io.ErrUnexpectedEOF, http.StatusBadRequest)
		return
	}
	a, _, err := v.Registry.Create(req.Engine, req.Label)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	jsonResponse(w, a)
}

func (v *V2) handleAccountRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil || v.Registry == nil {
		httpError(w, io.ErrUnexpectedEOF, http.StatusBadRequest)
		return
	}
	if err := v.Registry.Rename(r.PathValue("id"), req.Label); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (v *V2) handleAccountDelete(w http.ResponseWriter, r *http.Request) {
	if v.Registry == nil {
		httpError(w, io.ErrUnexpectedEOF, http.StatusBadRequest)
		return
	}
	if err := v.Registry.Remove(r.PathValue("id")); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (v *V2) handleAccountActive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Engine string `json:"engine"`
		ID     string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil || v.Registry == nil {
		httpError(w, io.ErrUnexpectedEOF, http.StatusBadRequest)
		return
	}
	if req.Engine == "agy" {
		if !strings.HasPrefix(req.ID, "agy@") {
			httpError(w, accounts.ErrNotFound, http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		switchProfile := func() error { return v.agyProfiles().Switch(ctx, strings.TrimPrefix(req.ID, "agy@")) }
		var err error
		if v.Mgr != nil {
			err = v.Mgr.ChangeEngineAccount("agy", switchProfile)
		} else {
			err = switchProfile()
		}
		if err != nil {
			httpError(w, err, http.StatusConflict)
			return
		}
		v.quotaMu.Lock()
		delete(v.quotaCache, "agy")
		v.quotaMu.Unlock()
		v.statusMu.Lock()
		delete(v.statusCache, "agy")
		v.statusMu.Unlock()
		v.modelMu.Lock()
		delete(v.modelCache, "agy")
		v.modelMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := v.Registry.SetActive(req.Engine, req.ID); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (v *V2) agyProfiles() *accounts.AGYProfiles {
	v.agyMu.Lock()
	defer v.agyMu.Unlock()
	if v.AGYProfiles == nil {
		v.AGYProfiles = &accounts.AGYProfiles{}
	}
	return v.AGYProfiles
}

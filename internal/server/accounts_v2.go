package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/nchungdev/agent-hub/internal/accounts"
	"github.com/nchungdev/agent-hub/internal/core"
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
	if err := v.Registry.SetActive(req.Engine, req.ID); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

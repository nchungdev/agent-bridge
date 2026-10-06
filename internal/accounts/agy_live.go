package accounts

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func (a *AGYProfiles) liveToken() string {
	return filepath.Join(a.home(), ".gemini", "antigravity-cli", "antigravity-oauth-token")
}

// LiveEmail is the Google account the agy CLI is really signed in as, read from the id_token of the live
// credential (profiles.json only records which snapshot was last switched to, and can drift from it).
func (a *AGYProfiles) LiveEmail() string {
	b, err := os.ReadFile(a.liveToken())
	if err != nil {
		return ""
	}
	var doc struct {
		IDToken string `json:"id_token"`
	}
	if json.Unmarshal(b, &doc) != nil {
		return ""
	}
	parts := strings.Split(doc.IDToken, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(payload, &claims)
	return strings.ToLower(claims.Email)
}

// Effective is List with Active corrected to the profile whose email matches the live login.
func (a *AGYProfiles) Effective() (AGYProfileList, error) {
	p, err := a.List()
	if err != nil {
		return p, err
	}
	live := a.LiveEmail()
	if live == "" {
		return p, nil
	}
	for _, x := range p.Profiles {
		if strings.ToLower(x.Email) == live {
			p.Active = x.ID
			return p, nil
		}
	}
	return p, nil
}

// Capture saves the live agy login as a profile (refreshing the one with the same email, else adding
// one) and marks it active. It is how an account signed in with the agy CLI gets added from the GUI.
func (a *AGYProfiles) Capture() (AGYProfile, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	email := a.LiveEmail()
	if email == "" {
		return AGYProfile{}, errors.New("agy is not signed in: run `agy` once and sign in, then retry")
	}
	p, err := a.List()
	if err != nil {
		return AGYProfile{}, err
	}
	prof := AGYProfile{ID: slugRe.ReplaceAllString(strings.SplitN(email, "@", 2)[0], "_"), Name: email, Email: email}
	exists := false
	for _, x := range p.Profiles {
		if strings.ToLower(x.Email) == email {
			prof, exists = x, true
		}
	}
	if !exists {
		for taken := true; taken; {
			taken = false
			for _, x := range p.Profiles {
				taken = taken || x.ID == prof.ID
			}
			if taken {
				prof.ID += "_"
			}
		}
	}
	root := filepath.Join(a.home(), ".gemini")
	dir := filepath.Join(root, "profiles", prof.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return prof, err
	}
	for _, f := range [][2]string{
		{filepath.Join(root, "antigravity-cli", "antigravity-oauth-token"), "antigravity-oauth-token"},
		{filepath.Join(root, "oauth_creds.json"), "oauth_creds.json"},
		{filepath.Join(root, "google_accounts.json"), "google_accounts.json"},
	} {
		b, err := os.ReadFile(f[0])
		if os.IsNotExist(err) && f[1] != "antigravity-oauth-token" {
			continue
		}
		if err != nil {
			return prof, err
		}
		if err := atomicPrivate(filepath.Join(dir, f[1]), b); err != nil {
			return prof, err
		}
	}
	document := map[string]json.RawMessage{}
	if b, err := os.ReadFile(a.path()); err == nil {
		if err := json.Unmarshal(b, &document); err != nil {
			return prof, err
		}
	}
	if !exists {
		p.Profiles = append(p.Profiles, prof)
	}
	document["profiles"], _ = json.Marshal(p.Profiles)
	document["active_profile"], _ = json.Marshal(prof.ID)
	b, err := json.MarshalIndent(document, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(a.path()), 0o700)
	}
	if err == nil {
		err = atomicPrivate(a.path(), b)
	}
	return prof, err
}

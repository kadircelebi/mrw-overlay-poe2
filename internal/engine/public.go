package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"poe2filter/internal/filter"
	"poe2filter/internal/i18n"
	"poe2filter/internal/publicprofile"
)

// Followed is a published profile someone else made, as last downloaded.
type Followed struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"` // the author's name for it
	Author   string          `json:"author"`
	Version  int             `json:"version"`
	Document json.RawMessage `json:"document"`
	// Gone means the server no longer has it (deleted or hidden); the last
	// copy keeps working.
	Gone bool `json:"gone,omitempty"`
	// Counted is set once the server counted this install as a follower;
	// until then the follow is retried with each check.
	Counted bool `json:"counted,omitempty"`
}

// FollowInfo is Followed without the document, for the panel.
type FollowInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Author  string `json:"author"`
	Version int    `json:"version"`
	Gone    bool   `json:"gone"`
}

func (f *Followed) document() (publicprofile.Document, bool) {
	if f == nil {
		return publicprofile.Document{}, false
	}
	doc, err := publicprofile.Decode(f.Document)
	return doc, err == nil
}

// lockFollowed keeps a followed profile's filter settings the author's: an
// edit in the panel is put back, while the player's own settings (league,
// language, scanning...) still change.
func (e *Engine) lockFollowed(c filter.Config) filter.Config {
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	if i := st.indexOf(st.Active); i >= 0 {
		if doc, ok := st.Profiles[i].Follow.document(); ok {
			return doc.Apply(c)
		}
	}
	return c
}

// SharedProfile is what publishing name would send, with the groups whose
// own sound file is left out. A followed profile cannot be published again.
func (e *Engine) SharedProfile(name string) (publicprofile.Document, []publicprofile.SoundSwap, string, error) {
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	i := st.indexOf(name)
	if i < 0 {
		return publicprofile.Document{}, nil, "", fmt.Errorf(i18n.T("err.profileMissing"), name)
	}
	p := st.Profiles[i]
	if p.Follow != nil {
		return publicprofile.Document{}, nil, "", errors.New(i18n.T("err.publishFollowed"))
	}
	cfg := p.Config
	if strings.EqualFold(p.Name, st.Active) {
		cfg = e.Config()
	}
	doc, swaps := publicprofile.FromConfig(cfg)
	return doc, swaps, p.PublicID, nil
}

// SetPublicID records (or with "" forgets) the server id of a profile this
// install published.
func (e *Engine) SetPublicID(name, id string) error {
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	i := st.indexOf(name)
	if i < 0 {
		return fmt.Errorf(i18n.T("err.profileMissing"), name)
	}
	st.Profiles[i].PublicID = id
	return e.saveProfiles(st)
}

// AddFollowed stores a downloaded profile as a followed one and returns its
// local name. Following the same profile twice returns the existing one.
// The player's own settings are taken from the current config.
func (e *Engine) AddFollowed(l publicprofile.Listing, doc publicprofile.Document) (string, error) {
	raw, err := publicprofile.Encode(doc)
	if err != nil {
		return "", err
	}
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	for _, p := range st.Profiles {
		if p.Follow != nil && p.Follow.ID == l.ID {
			return p.Name, nil
		}
	}
	if len(st.Profiles) >= MaxProfiles {
		return "", fmt.Errorf(i18n.T("err.profileLimit"), MaxProfiles)
	}
	author, _, _ := strings.Cut(l.Author, "#")
	base := filter.CleanText(l.Name + " · " + author)
	name := base
	for n := 2; st.indexOf(name) >= 0; n++ {
		name = fmt.Sprintf("%s (%d)", base, n)
	}
	if cur := st.indexOf(st.Active); cur >= 0 {
		st.Profiles[cur].Config = e.Config()
	}
	st.Profiles = append(st.Profiles, Profile{
		Name:   name,
		Config: doc.Apply(e.Config()),
		Follow: &Followed{ID: l.ID, Name: l.Name, Author: l.Author, Version: l.Version, Document: raw},
	})
	return name, e.saveProfiles(st)
}

// UncountedFollows are followed profiles the server has not counted yet.
func (e *Engine) UncountedFollows() []string {
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	var out []string
	for _, p := range e.loadProfiles().Profiles {
		if p.Follow != nil && !p.Follow.Counted && !p.Follow.Gone {
			out = append(out, p.Follow.ID)
		}
	}
	return out
}

// SetFollowCounted records that the server counted the follow of id.
func (e *Engine) SetFollowCounted(id string) error {
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	for i := range st.Profiles {
		if f := st.Profiles[i].Follow; f != nil && f.ID == id {
			f.Counted = true
		}
	}
	return e.saveProfiles(st)
}

// FollowedVersions maps each followed profile's server id to the version
// held here.
func (e *Engine) FollowedVersions() map[string]int {
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	out := map[string]int{}
	for _, p := range e.loadProfiles().Profiles {
		if p.Follow != nil {
			out[p.Follow.ID] = p.Follow.Version
		}
	}
	return out
}

// FollowedUpdate is a followed profile's new state from the server; a nil
// Document means it was removed there.
type FollowedUpdate struct {
	Listing  publicprofile.Listing
	Document *publicprofile.Document
}

// UpdateFollowed applies what the server said about one followed profile.
// When the active profile changed, it returns the config to apply (the
// caller sets it, which rewrites the filter).
func (e *Engine) UpdateFollowed(id string, u FollowedUpdate) (active *filter.Config, name string, err error) {
	var raw []byte
	if u.Document != nil {
		if raw, err = publicprofile.Encode(*u.Document); err != nil {
			return nil, "", err
		}
	}
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	for i := range st.Profiles {
		p := &st.Profiles[i]
		if p.Follow == nil || p.Follow.ID != id {
			continue
		}
		name = p.Name
		if u.Document == nil {
			p.Follow.Gone = true
			break
		}
		p.Follow.Gone = false
		p.Follow.Name, p.Follow.Author, p.Follow.Version, p.Follow.Document = u.Listing.Name, u.Listing.Author, u.Listing.Version, raw
		if strings.EqualFold(p.Name, st.Active) {
			cfg := u.Document.Apply(e.Config())
			p.Config, active = cfg, &cfg
		} else {
			p.Config = u.Document.Apply(p.Config)
		}
	}
	if name == "" {
		return nil, "", nil
	}
	return active, name, e.saveProfiles(st)
}

package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"poe2filter/internal/filter"
	"poe2filter/internal/i18n"
	"poe2filter/internal/prices"
)

// A profile is a complete settings set with a name. Players farm different
// content with different filters, and switching should be one click rather
// than a dozen sliders. A profile carries the whole config on purpose: that
// makes the exported file something a friend can simply load.

// ProfileShareVersion is the format of an exported profile file.
const ProfileShareVersion = 1

// MaxProfiles keeps the picker (and the settings folder) sane.
const MaxProfiles = 20

// Profile is one saved settings set.
type Profile struct {
	Name   string        `json:"name"`
	Config filter.Config `json:"config"`
}

// ProfileInfo is what the panel needs to draw the picker.
type ProfileInfo struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ProfileShare is an exported profile.
type ProfileShare struct {
	Version int           `json:"version"`
	Name    string        `json:"name"`
	Config  filter.Config `json:"config"`
}

type profileStore struct {
	Version  int       `json:"version"`
	Active   string    `json:"active"`
	Profiles []Profile `json:"profiles"`
}

func (e *Engine) profilesPath() string { return filepath.Join(e.opt.Dir, "profiles.json") }

// loadProfiles reads the store, creating the first profile from the current
// config so there is always exactly one active profile.
func (e *Engine) loadProfiles() profileStore {
	var st profileStore
	if data, err := os.ReadFile(e.profilesPath()); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	if st.Version != ProfileShareVersion || len(st.Profiles) == 0 {
		st = profileStore{
			Version:  ProfileShareVersion,
			Active:   i18n.T("profile.default"),
			Profiles: []Profile{{Name: i18n.T("profile.default"), Config: e.Config()}},
		}
	}
	if st.indexOf(st.Active) < 0 {
		st.Active = st.Profiles[0].Name
	}
	return st
}

func (s profileStore) indexOf(name string) int {
	for i, p := range s.Profiles {
		if strings.EqualFold(p.Name, name) {
			return i
		}
	}
	return -1
}

func (e *Engine) saveProfiles(st profileStore) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return prices.WriteFileAtomic(e.profilesPath(), data)
}

// syncActiveProfile copies the live config into the active profile, so the
// profile always reflects what the user last changed.
func (e *Engine) syncActiveProfile() {
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	if i := st.indexOf(st.Active); i >= 0 {
		st.Profiles[i].Config = e.Config()
		_ = e.saveProfiles(st)
	}
}

// Profiles lists the saved settings sets in their own order.
func (e *Engine) Profiles() []ProfileInfo {
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	out := make([]ProfileInfo, 0, len(st.Profiles))
	for _, p := range st.Profiles {
		out = append(out, ProfileInfo{Name: p.Name, Active: strings.EqualFold(p.Name, st.Active)})
	}
	return out
}

// SaveProfileAs stores the current settings under a new name and makes it
// active, leaving the profile it was copied from untouched.
func (e *Engine) SaveProfileAs(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New(i18n.T("err.profileName"))
	}
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	if i := st.indexOf(st.Active); i >= 0 {
		st.Profiles[i].Config = e.Config() // keep the one we are leaving current
	}
	if i := st.indexOf(name); i >= 0 {
		st.Profiles[i].Config = e.Config()
	} else {
		if len(st.Profiles) >= MaxProfiles {
			return fmt.Errorf(i18n.T("err.profileLimit"), MaxProfiles)
		}
		st.Profiles = append(st.Profiles, Profile{Name: name, Config: e.Config()})
	}
	st.Active = name
	return e.saveProfiles(st)
}

// RenameProfile changes only the profile's display name. Its settings and
// position stay intact, and renaming the active profile keeps it active.
func (e *Engine) RenameProfile(oldName, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errors.New(i18n.T("err.profileName"))
	}
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	i := st.indexOf(oldName)
	if i < 0 {
		return fmt.Errorf(i18n.T("err.profileMissing"), oldName)
	}
	if existing := st.indexOf(newName); existing >= 0 && existing != i {
		return fmt.Errorf(i18n.T("err.profileExists"), newName)
	}
	wasActive := strings.EqualFold(st.Profiles[i].Name, st.Active)
	if wasActive {
		st.Profiles[i].Config = e.Config()
		st.Active = newName
	}
	st.Profiles[i].Name = newName
	return e.saveProfiles(st)
}

// SwitchProfile makes another profile current and returns its settings. The
// caller applies them, which is what actually rewrites the filter.
func (e *Engine) SwitchProfile(name string) (filter.Config, error) {
	e.profileMu.Lock()
	st := e.loadProfiles()
	i := st.indexOf(name)
	if i < 0 {
		e.profileMu.Unlock()
		return e.Config(), fmt.Errorf(i18n.T("err.profileMissing"), name)
	}
	if cur := st.indexOf(st.Active); cur >= 0 {
		st.Profiles[cur].Config = e.Config() // do not lose edits made since the switch
	}
	st.Active = st.Profiles[i].Name
	cfg := st.Profiles[i].Config
	err := e.saveProfiles(st)
	e.profileMu.Unlock()
	if err != nil {
		return e.Config(), err
	}
	return cfg, nil
}

// DeleteProfile removes a profile. The active one can only go when another
// profile is there to take over, which the caller then switches to.
func (e *Engine) DeleteProfile(name string) (string, error) {
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	i := st.indexOf(name)
	if i < 0 {
		return st.Active, fmt.Errorf(i18n.T("err.profileMissing"), name)
	}
	if len(st.Profiles) == 1 {
		return st.Active, errors.New(i18n.T("err.profileLast"))
	}
	wasActive := strings.EqualFold(st.Profiles[i].Name, st.Active)
	st.Profiles = append(st.Profiles[:i], st.Profiles[i+1:]...)
	if wasActive {
		st.Active = st.Profiles[0].Name
	}
	return st.Active, e.saveProfiles(st)
}

// ExportProfile returns a profile as a file another player can import.
func (e *Engine) ExportProfile(name string) ([]byte, error) {
	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	i := st.indexOf(name)
	if i < 0 {
		return nil, fmt.Errorf(i18n.T("err.profileMissing"), name)
	}
	cfg := st.Profiles[i].Config
	if strings.EqualFold(st.Profiles[i].Name, st.Active) {
		cfg = e.Config() // the active one may have unsaved edits
	}
	return json.MarshalIndent(ProfileShare{Version: ProfileShareVersion, Name: st.Profiles[i].Name, Config: cfg}, "", "  ")
}

// ImportProfile adds a profile from a file and returns the name it was stored
// under; a clashing name gets a number so nothing is overwritten silently.
func (e *Engine) ImportProfile(data []byte) (string, error) {
	var in ProfileShare
	if err := json.Unmarshal(data, &in); err != nil {
		return "", fmt.Errorf(i18n.T("err.importBroken"), err)
	}
	if in.Version != ProfileShareVersion {
		return "", fmt.Errorf(i18n.T("err.importVersion"), in.Version, ProfileShareVersion)
	}
	in.Config.Normalize()

	e.profileMu.Lock()
	defer e.profileMu.Unlock()
	st := e.loadProfiles()
	if len(st.Profiles) >= MaxProfiles {
		return "", fmt.Errorf(i18n.T("err.profileLimit"), MaxProfiles)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = i18n.T("profile.imported")
	}
	base := name
	for n := 2; st.indexOf(name) >= 0; n++ {
		name = fmt.Sprintf("%s (%d)", base, n)
	}
	if cur := st.indexOf(st.Active); cur >= 0 {
		st.Profiles[cur].Config = e.Config()
	}
	st.Profiles = append(st.Profiles, Profile{Name: name, Config: in.Config})
	return name, e.saveProfiles(st)
}

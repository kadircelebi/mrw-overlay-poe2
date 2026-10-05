// Package uitheme owns application appearance independently of loot filters.
package uitheme

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

//go:embed presets.json
var presetJSON []byte

type Theme struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Base     string            `json:"base"`
	Colors   map[string]string `json:"colors"`
	Grain    bool              `json:"grain"`
	ReadOnly bool              `json:"readOnly"`
}
type State struct {
	Version  int     `json:"version"`
	Revision uint64  `json:"revision"`
	Selected string  `json:"selected"`
	Themes   []Theme `json:"themes"`
}
type document struct {
	Version  int     `json:"version"`
	Revision uint64  `json:"revision"`
	Selected string  `json:"selected"`
	Custom   []Theme `json:"custom"`
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var presets = func() []Theme {
	var out []Theme
	if err := json.Unmarshal(presetJSON, &out); err != nil {
		panic(err)
	}
	return out
}()

type Store struct {
	mu      sync.Mutex
	path    string
	doc     document
	loadErr error
}

func New(path string) *Store {
	s := &Store{path: path, doc: document{Version: 1, Selected: "default", Custom: []Theme{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err != nil {
		s.loadErr = err
		return s
	}
	var d document
	if err = json.Unmarshal(b, &d); err == nil {
		err = validateDocument(d)
	}
	if err != nil {
		s.loadErr = fmt.Errorf("cannot load UI themes: %w", err)
		return s
	}
	s.doc = d
	return s
}
func builtin(id string) (Theme, bool) {
	for _, t := range presets {
		if t.ID == id {
			return t, true
		}
	}
	return Theme{}, false
}
func clone(t Theme) Theme {
	colors := map[string]string{}
	for k, v := range t.Colors {
		colors[k] = v
	}
	t.Colors = colors
	return t
}
func validateTheme(t Theme) error {
	base, ok := builtin(t.Base)
	if !ok {
		return errors.New("unknown base theme")
	}
	if t.ID == "" {
		return errors.New("missing theme id")
	}
	if _, ok := builtin(t.ID); ok {
		return errors.New("built-in themes cannot be overwritten")
	}
	name := strings.TrimSpace(t.Name)
	if name == "" || utf8.RuneCountInString(name) > 60 {
		return errors.New("theme name must contain 1 to 60 characters")
	}
	for _, p := range presets {
		if strings.EqualFold(name, p.Name) {
			return errors.New("this name belongs to a built-in theme")
		}
	}
	if len(t.Colors) != len(base.Colors) {
		return errors.New("incomplete theme palette")
	}
	for k, v := range t.Colors {
		if _, ok := base.Colors[k]; !ok || !colorPattern.MatchString(v) {
			return fmt.Errorf("invalid theme color: %s", k)
		}
	}
	if t.ReadOnly {
		return errors.New("custom themes cannot be marked as built-in")
	}
	return nil
}
func validateDocument(d document) error {
	if d.Version != 1 {
		return errors.New("unsupported UI theme version")
	}
	if len(d.Custom) > 50 {
		return errors.New("too many custom themes")
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	for _, t := range d.Custom {
		if err := validateTheme(t); err != nil {
			return err
		}
		n := strings.ToLower(strings.TrimSpace(t.Name))
		if ids[t.ID] || names[n] {
			return errors.New("duplicate theme")
		}
		ids[t.ID] = true
		names[n] = true
	}
	if _, ok := builtin(d.Selected); !ok && !ids[d.Selected] {
		return errors.New("selected theme does not exist")
	}
	return nil
}
func (s *Store) state() State {
	out := State{Version: 1, Revision: s.doc.Revision, Selected: s.doc.Selected, Themes: []Theme{}}
	for _, p := range presets {
		out.Themes = append(out.Themes, clone(p))
	}
	for _, t := range s.doc.Custom {
		out.Themes = append(out.Themes, clone(t))
	}
	return out
}
func (s *Store) Get() (State, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.state(), s.loadErr }
func (s *Store) commit(d document) (State, error) {
	if s.loadErr != nil {
		return s.state(), s.loadErr
	}
	if err := validateDocument(d); err != nil {
		return s.state(), err
	}
	d.Revision = s.doc.Revision + 1
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return s.state(), err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return s.state(), err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".ui-themes-*")
	if err != nil {
		return s.state(), err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return s.state(), err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return s.state(), err
	}
	if err = f.Close(); err != nil {
		return s.state(), err
	}
	if err = os.Rename(name, s.path); err != nil {
		return s.state(), err
	}
	s.doc = d
	return s.state(), nil
}
func (s *Store) find(id string) (Theme, bool) {
	if t, ok := builtin(id); ok {
		return t, true
	}
	for _, t := range s.doc.Custom {
		if t.ID == id {
			return t, true
		}
	}
	return Theme{}, false
}
func (s *Store) Create(sourceID, name string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, ok := s.find(sourceID)
	if !ok {
		return s.state(), errors.New("source theme does not exist")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return s.state(), err
	}
	t := clone(source)
	t.ID = "custom-" + hex.EncodeToString(b)
	t.Name = strings.TrimSpace(name)
	t.ReadOnly = false
	d := s.doc
	d.Custom = append(append([]Theme{}, d.Custom...), t)
	d.Selected = t.ID
	return s.commit(d)
}
func (s *Store) Save(t Theme) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := builtin(t.ID); ok {
		return s.state(), errors.New("built-in themes cannot be overwritten")
	}
	old, ok := s.find(t.ID)
	if !ok {
		return s.state(), errors.New("theme does not exist")
	}
	if t.Base != old.Base {
		return s.state(), errors.New("a theme's base cannot be changed")
	}
	t = clone(t)
	t.Name = strings.TrimSpace(t.Name)
	for k, v := range t.Colors {
		t.Colors[k] = strings.ToLower(v)
	}
	d := s.doc
	d.Custom = append([]Theme{}, d.Custom...)
	for i := range d.Custom {
		if d.Custom[i].ID == t.ID {
			d.Custom[i] = t
		}
	}
	return s.commit(d)
}
func (s *Store) Select(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.find(id); !ok {
		return s.state(), errors.New("theme does not exist")
	}
	d := s.doc
	d.Selected = id
	return s.commit(d)
}
func (s *Store) Delete(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := builtin(id); ok {
		return s.state(), errors.New("built-in themes cannot be deleted")
	}
	t, ok := s.find(id)
	if !ok {
		return s.state(), errors.New("theme does not exist")
	}
	d := s.doc
	d.Custom = []Theme{}
	for _, v := range s.doc.Custom {
		if v.ID != id {
			d.Custom = append(d.Custom, v)
		}
	}
	if d.Selected == id {
		d.Selected = t.Base
	}
	return s.commit(d)
}

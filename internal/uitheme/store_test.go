package uitheme

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestBuiltinsAreProtected(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "ui_themes.json"))
	before, _ := s.Get()
	for _, p := range before.Themes {
		changed := clone(p)
		changed.Name = "Changed"
		changed.ReadOnly = false
		changed.Colors["bg"] = "#ffffff"
		if _, err := s.Save(changed); err == nil {
			t.Fatalf("overwrote %s", p.ID)
		}
		if _, err := s.Delete(p.ID); err == nil {
			t.Fatalf("deleted %s", p.ID)
		}
		if _, err := s.Create("default", p.Name); err == nil {
			t.Fatalf("reserved name %s accepted", p.Name)
		}
	}
	after, _ := s.Get()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed writes changed state")
	}
	before.Themes[0].Colors["bg"] = "#ffffff"
	fresh, _ := s.Get()
	if fresh.Themes[0].Colors["bg"] == "#ffffff" {
		t.Fatal("Get exposed shared palette map")
	}
}
func TestDeriveEditReloadAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui_themes.json")
	s := New(path)
	state, err := s.Create("light", "Kremim")
	if err != nil {
		t.Fatal(err)
	}
	custom := state.Themes[3]
	if custom.ReadOnly || custom.Base != "light" || !reflect.DeepEqual(custom.Colors, state.Themes[2].Colors) {
		t.Fatal("copy did not inherit source")
	}
	custom.Colors["gold"] = "#123456"
	custom.Name = "Deniz"
	state, err = s.Save(custom)
	if err != nil {
		t.Fatal(err)
	}
	if state.Themes[2].Colors["gold"] == "#123456" {
		t.Fatal("edited built-in via copy")
	}
	derived, err := s.Create(custom.ID, "Deniz 2")
	if err != nil {
		t.Fatal(err)
	}
	if derived.Themes[4].Colors["gold"] != "#123456" {
		t.Fatal("custom source not inherited")
	}
	reload, err := New(path).Get()
	if err != nil || !reflect.DeepEqual(reload, derived) {
		t.Fatalf("reload: %v", err)
	}
	state, err = s.Delete(derived.Selected)
	if err != nil {
		t.Fatal(err)
	}
	if state.Selected != "light" {
		t.Fatal("selected deletion did not fall back to built-in")
	}
	if len(state.Themes) != 4 {
		t.Fatal("wrong theme deleted")
	}
	if _, err = s.Select(custom.ID); err != nil {
		t.Fatal(err)
	}
}
func TestInvalidEditsNeverPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui_themes.json")
	s := New(path)
	state, _ := s.Create("dark", "Mine")
	before, _ := os.ReadFile(path)
	cases := map[string]func(*Theme){
		"unknown role":  func(t *Theme) { t.Colors["crafted"] = "#abcdef" },
		"missing role":  func(t *Theme) { delete(t.Colors, "bg") },
		"css injection": func(t *Theme) { t.Colors["bg"] = "red; background:url(x)" },
		"empty name":    func(t *Theme) { t.Name = "  " },
		"reserved name": func(t *Theme) { t.Name = "LIGHT" },
		"changed base":  func(t *Theme) { t.Base = "light" },
		"readonly":      func(t *Theme) { t.ReadOnly = true },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			next := clone(state.Themes[3])
			change(&next)
			if _, err := s.Save(next); err == nil {
				t.Fatal("invalid edit accepted")
			}
			b, _ := os.ReadFile(path)
			if string(b) != string(before) {
				t.Fatal("failed edit changed file")
			}
		})
	}
	if _, err := s.Create("missing", "Name"); err == nil {
		t.Fatal("missing source accepted")
	}
	if _, err := s.Create("dark", "mine"); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if _, err := s.Select("missing"); err == nil {
		t.Fatal("missing selection accepted")
	}
}
func TestCorruptFileIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui_themes.json")
	bad := []byte(`{"version":99}`)
	os.WriteFile(path, bad, 0600)
	s := New(path)
	if _, err := s.Get(); err == nil {
		t.Fatal("corruption hidden")
	}
	if _, err := s.Create("default", "Mine"); err == nil {
		t.Fatal("corrupt file overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(bad) {
		t.Fatal("corrupt file changed")
	}
}
func TestFailedDiskWriteKeepsPreviousState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "themes.json")
	s := New(path)
	before, _ := s.Get()
	os.WriteFile(filepath.Join(dir, "blocked"), []byte("file"), 0600)
	s.path = filepath.Join(dir, "blocked", "themes.json")
	if _, err := s.Create("light", "Mine"); err == nil {
		t.Fatal("write failure hidden")
	}
	after, _ := s.Get()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed write changed state")
	}
}
func TestConcurrentCopiesAreNotLost(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "ui_themes.json"))
	var wg sync.WaitGroup
	for i, name := range []string{"A", "B", "C", "D", "E", "F"} {
		_ = i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Create("light", name); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	state, _ := s.Get()
	if len(state.Themes) != 9 || state.Revision != 6 {
		t.Fatal("concurrent changes lost")
	}
}

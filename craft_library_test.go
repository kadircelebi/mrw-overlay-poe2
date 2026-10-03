package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCraftLibraryRoundTrip(t *testing.T) {
	s := &AppService{meta: Meta{DataDir: t.TempDir()}}
	if got, err := s.CraftLibrary(); err != nil || got != "[]" {
		t.Fatalf("empty library = %q, %v", got, err)
	}
	if err := s.SaveCraftLibrary(`[{"id":"a","name":"Wand"}]`); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.CraftLibrary(); got != `[{"id":"a","name":"Wand"}]` {
		t.Fatalf("library = %q", got)
	}
	if err := s.SaveCraftLibrary(`{"not":"a list"}`); err == nil {
		t.Fatal("an object was accepted as the library")
	}
	// A damaged file is set aside, not handed to the page.
	os.WriteFile(filepath.Join(s.meta.DataDir, craftLibraryFile), []byte("[{broken"), 0o644)
	if got, _ := s.CraftLibrary(); got != "[]" {
		t.Fatalf("damaged library = %q", got)
	}
	if _, err := os.Stat(filepath.Join(s.meta.DataDir, craftLibraryFile+".damaged")); err != nil {
		t.Fatal("damaged library was not kept aside")
	}
}

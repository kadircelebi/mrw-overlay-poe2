package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopySoundIntoFilterFolder(t *testing.T) {
	src := filepath.Join(t.TempDir(), "horn.mp3")
	if err := os.WriteFile(src, []byte("ID3 pretend audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := t.TempDir()

	name, err := copySoundInto(src, game)
	if err != nil {
		t.Fatal(err)
	}
	if name != "horn.mp3" {
		t.Errorf("the game knows the sound by its file name, got %q", name)
	}
	got, err := os.ReadFile(filepath.Join(game, "horn.mp3"))
	if err != nil || string(got) != "ID3 pretend audio" {
		t.Errorf("the file should have arrived intact: %v %q", err, got)
	}
}

// Picking a file that is already in the filter folder is a normal thing to do;
// copying it onto itself would empty it.
func TestCopySoundKeepsAFileAlreadyThere(t *testing.T) {
	game := t.TempDir()
	path := filepath.Join(game, "horn.mp3")
	if err := os.WriteFile(path, []byte("ID3 pretend audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := copySoundInto(path, game); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "ID3 pretend audio" {
		t.Errorf("the file was truncated: %q", got)
	}
}

func TestCopySoundRejectsOtherFileTypes(t *testing.T) {
	src := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := t.TempDir()
	if _, err := copySoundInto(src, game); err == nil {
		t.Error("only mp3/wav belong in the filter folder")
	}
	if files, _ := filepath.Glob(filepath.Join(game, "*")); len(files) != 0 {
		t.Errorf("nothing should have been written: %v", files)
	}
}

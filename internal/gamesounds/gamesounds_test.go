package gamesounds

import (
	"os"
	"path/filepath"
	"testing"
)

// A missing or misspelled file would only show up as a silent play button on
// someone else's machine, so check the set here instead.
func TestEveryIDHasAFile(t *testing.T) {
	entries, err := files.ReadDir("files")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(IDs) {
		t.Errorf("embedded %d files for %d sounds", len(entries), len(IDs))
	}
	for _, id := range IDs {
		data, err := files.ReadFile("files/" + name(id))
		if err != nil {
			t.Errorf("sound %q has no file: %v", id, err)
			continue
		}
		if len(data) < 4<<10 {
			t.Errorf("sound %q is only %d bytes, which is not an alert sound", id, len(data))
		}
		if string(data[:3]) != "ID3" && !(data[0] == 0xFF && data[1]&0xE0 == 0xE0) {
			t.Errorf("sound %q is not an mp3", id)
		}
	}
}

func TestEnsureWritesTheSoundOnce(t *testing.T) {
	dir := t.TempDir()
	path, err := Ensure(dir, "ShMirror")
	if err != nil {
		t.Fatal(err)
	}
	if path != Path(dir, "ShMirror") {
		t.Errorf("unexpected path %q", path)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() < 4<<10 {
		t.Fatalf("the file should be on disk now: %v %v", err, info)
	}

	// A file that is already there is left alone, including one a user put
	// there themselves or an older version downloaded.
	if err := os.WriteFile(path, []byte("older copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(dir, "ShMirror"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "older copy" {
		t.Error("an existing file must not be overwritten")
	}
}

func TestEnsureRefusesAnUnknownID(t *testing.T) {
	dir := t.TempDir()
	if _, err := Ensure(dir, "27"); err == nil {
		t.Error("only the game's own sounds have files")
	}
	if files, _ := filepath.Glob(filepath.Join(Dir(dir), "*")); len(files) != 0 {
		t.Errorf("nothing should have been written: %v", files)
	}
}

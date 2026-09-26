package overlay

import (
	"os"
	"path/filepath"
	"testing"

	"poe2filter/internal/trade"
)

func TestSearchStoreRoundTrip(t *testing.T) {
	store := NewSearchStore(filepath.Join(t.TempDir(), "overlay_searches.json"))
	lib, err := store.Save("Mageblood exact", "", trade.EvaluateRequest{BaseType: "Utility Belt", Name: "Mageblood", Rarity: "unique"})
	if err != nil || len(lib.Searches) != 1 {
		t.Fatalf("save = %#v, %v", lib, err)
	}
	loaded, err := store.List()
	if err != nil || len(loaded.Searches) != 1 || loaded.Searches[0].Query.Name != "Mageblood" {
		t.Fatalf("list = %#v, %v", loaded, err)
	}
	loaded, err = store.Delete(loaded.Searches[0].ID)
	if err != nil || len(loaded.Searches) != 0 {
		t.Fatalf("delete = %#v, %v", loaded, err)
	}
}

func TestSearchStoreRejectsIncompleteSearch(t *testing.T) {
	store := NewSearchStore(filepath.Join(t.TempDir(), "overlay_searches.json"))
	if _, err := store.Save("", "", trade.EvaluateRequest{BaseType: "Boots"}); err == nil {
		t.Fatal("empty name should fail")
	}
	if _, err := store.Save("Missing base", "", trade.EvaluateRequest{}); err == nil {
		t.Fatal("empty base type should fail")
	}
}

// Before folders the file was a bare array; those saves must keep loading.
func TestSearchStoreReadsTheOldArrayFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overlay_searches.json")
	old := `[{"id":"a1","name":"Mageblood","query":{"baseType":"Utility Belt","name":"Mageblood"},"createdAt":"2026-09-25T10:00:00Z"}]`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewSearchStore(path)
	lib, err := store.List()
	if err != nil || len(lib.Searches) != 1 || lib.Searches[0].Name != "Mageblood" || len(lib.Folders) != 0 {
		t.Fatalf("old file = %#v, %v", lib, err)
	}
	// The next write moves the file to the new layout without losing it.
	if _, err := store.CreateFolder("Uniques"); err != nil {
		t.Fatal(err)
	}
	lib, err = store.List()
	if err != nil || len(lib.Searches) != 1 || len(lib.Folders) != 1 {
		t.Fatalf("after upgrade = %#v, %v", lib, err)
	}
}

func TestSearchFolders(t *testing.T) {
	store := NewSearchStore(filepath.Join(t.TempDir(), "overlay_searches.json"))
	lib, err := store.CreateFolder("Belts")
	if err != nil || len(lib.Folders) != 1 {
		t.Fatalf("create = %#v, %v", lib, err)
	}
	folder := lib.Folders[0].ID
	if _, err := store.CreateFolder("  "); err == nil {
		t.Error("an empty folder name should fail")
	}

	lib, _ = store.Save("Mageblood", folder, trade.EvaluateRequest{BaseType: "Utility Belt", Name: "Mageblood"})
	lib, _ = store.Save("Headhunter", "gone", trade.EvaluateRequest{BaseType: "Heavy Belt", Name: "Headhunter"})
	if lib.Searches[0].Folder != folder || lib.Searches[1].Folder != "" {
		t.Fatalf("a save lands in its folder, or at the top when the folder is unknown: %#v", lib.Searches)
	}

	lib, err = store.Move(lib.Searches[1].ID, folder)
	if err != nil || lib.Searches[1].Folder != folder {
		t.Fatalf("move = %#v, %v", lib.Searches, err)
	}
	if _, err := store.Move(lib.Searches[1].ID, "missing"); err == nil {
		t.Error("moving into a missing folder should fail")
	}

	lib, err = store.RenameFolder(folder, "Unique belts")
	if err != nil || lib.Folders[0].Name != "Unique belts" {
		t.Fatalf("rename = %#v, %v", lib.Folders, err)
	}

	// Deleting a folder keeps its searches, at the top level.
	lib, err = store.DeleteFolder(folder)
	if err != nil || len(lib.Folders) != 0 || len(lib.Searches) != 2 {
		t.Fatalf("delete folder = %#v, %v", lib, err)
	}
	for _, s := range lib.Searches {
		if s.Folder != "" {
			t.Errorf("%s should have moved to the top level", s.Name)
		}
	}
}

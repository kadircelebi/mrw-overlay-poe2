package filter

import (
	"path/filepath"
	"strings"
	"testing"
)

// OneDrive's folder backup moves Documents under the OneDrive folder; the old
// %USERPROFILE%\Documents lookup reported "folder not found" for those players.
func TestDocumentsDirsIncludesOneDrive(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OneDrive", root)
	t.Setenv("USERPROFILE", filepath.Join(root, "home"))
	dirs := documentsDirs()
	want := filepath.Join(root, "Documents")
	found := false
	for i, d := range dirs {
		if strings.EqualFold(d, want) {
			found = true
		}
		if strings.EqualFold(d, filepath.Join(root, "home", "Documents")) && !found {
			t.Fatalf("plain profile Documents (%d) listed before OneDrive: %v", i, dirs)
		}
	}
	if !found {
		t.Fatalf("OneDrive Documents missing: %v", dirs)
	}
}

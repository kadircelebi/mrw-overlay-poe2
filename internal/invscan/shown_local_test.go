package invscan

import (
	"os"
	"testing"
)

// TestLocalShown checks INVSCAN_SHOT's inventory grid is taken as shown
// (INVSCAN_SHOWN=0 when the picture has the inventory closed).
func TestLocalShown(t *testing.T) {
	want := os.Getenv("INVSCAN_SHOWN")
	if want == "" {
		t.Skip("INVSCAN_SHOWN not set")
	}
	pic, g := loadShot(t)
	if got := Shown(pic, g); got != (want != "0") {
		t.Errorf("Shown = %v, want %s", got, want)
	}
}

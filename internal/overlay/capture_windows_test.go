//go:build windows

package overlay

import (
	"testing"
	"unsafe"
)

// SendInput rejects the batch unless every record is exactly INPUT-sized.
func TestKeyInputIsAWindowsInputRecord(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("the app is built for 64-bit Windows only")
	}
	if got := unsafe.Sizeof(keyInput{}); got != 40 {
		t.Fatalf("keyInput is %d bytes, INPUT is 40", got)
	}
}

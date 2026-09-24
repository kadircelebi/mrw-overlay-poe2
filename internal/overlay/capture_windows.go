//go:build windows

package overlay

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procKeybdEvent   = user32.NewProc("keybd_event")
	procAsyncKey     = user32.NewProc("GetAsyncKeyState")
	procGetCursorPos = user32.NewProc("GetCursorPos")
)

const (
	vkMenu       = 0x12
	vkControl    = 0x11
	vkC          = 0x43
	vkE          = 0x45
	keyeventfUp  = 0x0002
	keyStateDown = 0x8000
)

func keyDown(vk uintptr) bool {
	r, _, _ := procAsyncKey.Call(vk)
	return uint16(r)&keyStateDown != 0
}

func keyEvent(vk uintptr, up bool) {
	flags := uintptr(0)
	if up {
		flags = keyeventfUp
	}
	_, _, _ = procKeybdEvent.Call(vk, 0, flags, 0)
}

// CopyAdvancedItem asks PoE to copy the hovered item with advanced modifier
// details. Alt is normally already held by the registered shortcut, but we
// synthesize it when the user released the key before the callback ran.
// refocused means the game only just got focus: it never saw the user press
// Alt, so the key is announced again.
func CopyAdvancedItem(refocused bool) error {
	deadline := time.Now().Add(220 * time.Millisecond)
	for keyDown(vkE) && time.Now().Before(deadline) {
		time.Sleep(8 * time.Millisecond)
	}
	pressedAlt := !keyDown(vkMenu)
	if pressedAlt || refocused {
		keyEvent(vkMenu, false)
		time.Sleep(15 * time.Millisecond)
	}
	keyEvent(vkControl, false)
	keyEvent(vkC, false)
	time.Sleep(12 * time.Millisecond)
	keyEvent(vkC, true)
	keyEvent(vkControl, true)
	if pressedAlt {
		keyEvent(vkMenu, true)
	}
	return nil
}

type point struct{ X, Y int32 }

func CursorPosition() (int, int, bool) {
	var p point
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y), r != 0
}

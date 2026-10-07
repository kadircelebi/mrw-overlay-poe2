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
	procSendInput    = user32.NewProc("SendInput")
)

const (
	vkMenu       = 0x12
	vkControl    = 0x11
	vkC          = 0x43
	vkE          = 0x45
	vkShift      = 0x10
	vkReturn     = 0x0D
	vkA          = 0x41
	vkV          = 0x56
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

// keyInput is a Windows INPUT record holding a KEYBDINPUT (40 bytes on
// 64-bit Windows: the type, then the union sized by MOUSEINPUT).
type keyInput struct {
	typ   uint32
	_     uint32
	vk    uint16
	scan  uint16
	flags uint32
	time  uint32
	_     uint32
	extra uintptr
	_     [8]byte
}

const inputKeyboard = 1

// sendKeys hands Windows the whole sequence in one SendInput call, so no
// other input lands in between and the game takes it within one frame.
func sendKeys(steps ...keyInput) bool {
	n, _, _ := procSendInput.Call(uintptr(len(steps)), uintptr(unsafe.Pointer(&steps[0])), unsafe.Sizeof(steps[0]))
	return int(n) == len(steps)
}

func down(vk uintptr) keyInput { return keyInput{typ: inputKeyboard, vk: uint16(vk)} }
func up(vk uintptr) keyInput   { return keyInput{typ: inputKeyboard, vk: uint16(vk), flags: keyeventfUp} }

// PasteChatLine sends the clipboard as one chat line, the way trade tools do:
// Enter opens the chat, Ctrl+A and Ctrl+V replace whatever was typed there,
// Enter sends it. It all goes in one batch, so the chat box is gone before
// the game draws it. The player's own modifiers are let go first, or Enter
// would arrive as Ctrl+Enter.
func PasteChatLine() {
	deadline := time.Now().Add(300 * time.Millisecond)
	for (keyDown(vkMenu) || keyDown(vkControl) || keyDown(vkShift)) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	sendKeys(
		down(vkReturn), up(vkReturn),
		down(vkControl), down(vkA), up(vkA), down(vkV), up(vkV), up(vkControl),
		down(vkReturn), up(vkReturn),
	)
}

type point struct{ X, Y int32 }

func CursorPosition() (int, int, bool) {
	var p point
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y), r != 0
}

// ClickOrEscape reports whether a mouse button or Escape is held down now,
// anywhere: the price labels lay over the game let clicks through, so they
// cannot see a click themselves.
func ClickOrEscape() bool {
	return keyDown(0x01) || keyDown(0x02) || keyDown(0x04) || keyDown(0x1B) // left, right, middle, Escape
}

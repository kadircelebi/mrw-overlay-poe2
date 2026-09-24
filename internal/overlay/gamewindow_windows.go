//go:build windows

package overlay

import (
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsIconic                 = user32.NewProc("IsIconic")
	procGetClientRect            = user32.NewProc("GetClientRect")
	procClientToScreen           = user32.NewProc("ClientToScreen")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procMonitorFromWindow        = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procSetWindowLongPtrW        = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW          = user32.NewProc("CallWindowProcW")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")

	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	gwlpWndProc             = ^uintptr(3) // -4
	wmSizing                = 0x0214
	wmMoving                = 0x0216
	wmEnterSizeMove         = 0x0231
	wmExitSizeMove          = 0x0232
	wmNCDestroy             = 0x0082
	swpNoSize               = 0x0001
	swpNoZOrder             = 0x0004
	swpNoActivate           = 0x0010
	monitorDefaultToNearest = 2
	processQueryLimitedInfo = 0x1000
	wmszLeft                = 1
	wmszRight               = 2
	wmszTop                 = 3
	wmszTopLeft             = 4
	wmszTopRight            = 5
	wmszBottom              = 6
	wmszBottomLeft          = 7
	wmszBottomRight         = 8
)

type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

var (
	gameProcMu sync.Mutex
	gameProcs  = map[uint32]bool{}
)

// ForegroundWindow returns the window that currently receives keyboard input.
func ForegroundWindow() uintptr {
	h, _, _ := procGetForegroundWindow.Call()
	return h
}

// WindowPID returns the process that owns hwnd, or 0.
func WindowPID(hwnd uintptr) uint32 {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// IsGameWindow reports whether hwnd belongs to a Path of Exile client
// (PathOfExile.exe, PathOfExileSteam.exe, PathOfExile_x64*.exe …).
func IsGameWindow(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	pid := WindowPID(hwnd)
	if pid == 0 {
		return false
	}
	gameProcMu.Lock()
	defer gameProcMu.Unlock()
	if v, ok := gameProcs[pid]; ok {
		return v
	}
	v := strings.HasPrefix(strings.ToLower(filepath.Base(processImage(pid))), "pathofexile")
	gameProcs[pid] = v
	return v
}

func processImage(pid uint32) string {
	h, err := syscall.OpenProcess(processQueryLimitedInfo, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

func visibleAndRestored(hwnd uintptr) bool {
	v, _, _ := procIsWindowVisible.Call(hwnd)
	i, _, _ := procIsIconic.Call(hwnd)
	return v != 0 && i == 0
}

var (
	enumMu   sync.Mutex
	enumBest uintptr
	enumArea int64
	enumCB   = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if visibleAndRestored(hwnd) && IsGameWindow(hwnd) {
			if r, ok := clientRect(hwnd); ok {
				if a := int64(r.width()) * int64(r.height()); a > enumArea {
					enumBest, enumArea = hwnd, a
				}
			}
		}
		return 1
	})
)

// gameWindow prefers the foreground game window and otherwise picks the
// largest visible one.
func gameWindow() uintptr {
	if fg := ForegroundWindow(); IsGameWindow(fg) && visibleAndRestored(fg) {
		return fg
	}
	enumMu.Lock()
	defer enumMu.Unlock()
	enumBest, enumArea = 0, 0
	procEnumWindows.Call(enumCB, 0)
	return enumBest
}

func clientRect(hwnd uintptr) (rect, bool) {
	var r rect
	if ok, _, _ := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return rect{}, false
	}
	tl := point{r.Left, r.Top}
	br := point{r.Right, r.Bottom}
	procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&tl)))
	procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&br)))
	out := rect{tl.X, tl.Y, br.X, br.Y}
	return out, out.width() > 0 && out.height() > 0
}

func windowRect(hwnd uintptr) (rect, bool) {
	var r rect
	ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r, ok != 0
}

// bounds is the area our window may occupy: the game's client area, or the
// work area of the window's own monitor when the game is not running.
func bounds(hwnd uintptr) rect {
	if g := gameWindow(); g != 0 {
		if r, ok := clientRect(g); ok {
			return r
		}
	}
	mon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	return mi.Work
}

// FocusGame gives keyboard focus back to the game so the synthesized copy
// shortcut reaches it and not our own window. switched reports that focus had
// to be moved; ok that the game is the foreground window afterwards.
func FocusGame() (switched, ok bool) {
	g := gameWindow()
	if g == 0 {
		return false, false
	}
	if ForegroundWindow() == g {
		return false, true
	}
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			// Windows refuses SetForegroundWindow unless the caller received
			// the last input event; a synthetic Alt tap satisfies that rule.
			keyEvent(vkMenu, false)
			keyEvent(vkMenu, true)
		}
		procSetForegroundWindow.Call(g)
		deadline := time.Now().Add(250 * time.Millisecond)
		for time.Now().Before(deadline) {
			if ForegroundWindow() == g {
				// Let the client process the activation before keys arrive.
				time.Sleep(80 * time.Millisecond)
				return true, true
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	return true, false
}

// GameCenter returns the physical centre of the game window.
func GameCenter() (int, int, bool) {
	g := gameWindow()
	if g == 0 {
		return 0, 0, false
	}
	r, ok := clientRect(g)
	if !ok {
		return 0, 0, false
	}
	return int(r.Left + r.width()/2), int(r.Top + r.height()/2), true
}

// shift moves r inside b without resizing; a window larger than b is pinned
// to b's top-left corner.
func shift(r *rect, b rect) {
	w, h := r.width(), r.height()
	switch {
	case w > b.width() || r.Left < b.Left:
		r.Left = b.Left
	case r.Right > b.Right:
		r.Left = b.Right - w
	}
	switch {
	case h > b.height() || r.Top < b.Top:
		r.Top = b.Top
	case r.Bottom > b.Bottom:
		r.Top = b.Bottom - h
	}
	r.Right, r.Bottom = r.Left+w, r.Top+h
}

// PlaceInGame moves hwnd inside the game window (centred when center is set)
// and shrinks it when the game window is smaller than it.
func PlaceInGame(hwnd uintptr, center bool) {
	if hwnd == 0 {
		return
	}
	r, ok := windowRect(hwnd)
	if !ok {
		return
	}
	b := bounds(hwnd)
	w, h := min(r.width(), b.width()), min(r.height(), b.height())
	if center {
		r.Left, r.Top = b.Left+(b.width()-w)/2, b.Top+(b.height()-h)/2
	}
	r.Right, r.Bottom = r.Left+w, r.Top+h
	shift(&r, b)
	flags := uintptr(swpNoZOrder | swpNoActivate)
	procSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()), flags)
}

type confined struct {
	prev   uintptr
	bounds rect
	active bool
}

var (
	confineMu sync.Mutex
	confines  = map[uintptr]*confined{}
	confineCB = syscall.NewCallback(confineProc)
)

// Confine keeps hwnd inside the game window while the user drags or resizes
// it. It subclasses the window procedure, so call it on the UI thread.
func Confine(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	confineMu.Lock()
	defer confineMu.Unlock()
	if _, ok := confines[hwnd]; ok {
		return
	}
	c := &confined{}
	confines[hwnd] = c
	c.prev, _, _ = procSetWindowLongPtrW.Call(hwnd, gwlpWndProc, confineCB)
}

func confineProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	confineMu.Lock()
	c := confines[hwnd]
	confineMu.Unlock()
	if c == nil {
		return 0
	}
	switch msg {
	case wmEnterSizeMove:
		c.bounds, c.active = bounds(hwnd), true
	case wmExitSizeMove:
		c.active = false
	case wmMoving:
		if c.active {
			shift((*rect)(unsafe.Pointer(lparam)), c.bounds)
		}
	case wmSizing:
		if c.active {
			clip((*rect)(unsafe.Pointer(lparam)), c.bounds, wparam)
		}
	case wmNCDestroy:
		confineMu.Lock()
		delete(confines, hwnd)
		confineMu.Unlock()
	}
	r, _, _ := procCallWindowProcW.Call(c.prev, hwnd, msg, wparam, lparam)
	if msg == wmMoving || msg == wmSizing {
		return 1
	}
	return r
}

// clip stops the dragged edges at the bounds.
func clip(r *rect, b rect, edge uintptr) {
	switch edge {
	case wmszLeft, wmszTopLeft, wmszBottomLeft:
		r.Left = max(r.Left, b.Left)
	}
	switch edge {
	case wmszRight, wmszTopRight, wmszBottomRight:
		r.Right = min(r.Right, b.Right)
	}
	switch edge {
	case wmszTop, wmszTopLeft, wmszTopRight:
		r.Top = max(r.Top, b.Top)
	}
	switch edge {
	case wmszBottom, wmszBottomLeft, wmszBottomRight:
		r.Bottom = min(r.Bottom, b.Bottom)
	}
}

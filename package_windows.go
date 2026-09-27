//go:build windows

package main

import (
	"html"
	"syscall"
	"unsafe"

	"git.sr.ht/~jackmordaunt/go-toast/v2/wintoast"
)

var procGetAUMID = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentApplicationUserModelId")

// packageAUMID returns the app user model ID Windows gave this process when
// it runs from an MSIX package (the Store build), or "" when it does not.
func packageAUMID() string {
	if procGetAUMID.Find() != nil {
		return ""
	}
	n := uint32(256)
	buf := make([]uint16, n)
	r, _, _ := procGetAUMID.Call(uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 { // APPMODEL_ERROR_NO_APPLICATION: not packaged
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// pushPackagedToast shows a toast under the package's own identity. Inside a
// package Windows silently drops toasts sent under any other ID, which is
// what Wails' notifier does (it uses the app name). Checked 2026-09-27: the
// same toast under the name vanished, under the package ID it appeared.
func pushPackagedToast(aumid, title, body string) error {
	xml := `<toast><visual><binding template="ToastGeneric"><text>` + html.EscapeString(title) +
		`</text><text>` + html.EscapeString(body) + `</text></binding></visual></toast>`
	return wintoast.Push(aumid, xml)
}

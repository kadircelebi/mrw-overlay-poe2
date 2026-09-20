//go:build windows

package i18n

import (
	"strings"
	"syscall"
	"unsafe"
)

// detect reads the Windows display language. Turkish and Traditional Chinese
// get their own interface; everything else falls back to English.
func detect() Lang {
	return fromTag(userLocale())
}

func fromTag(tag string) Lang {
	t := strings.ToLower(strings.ReplaceAll(tag, "_", "-"))
	switch {
	case strings.HasPrefix(t, "tr"):
		return TR
	case t == "zh-tw" || t == "zh-hk" || t == "zh-mo" || strings.Contains(t, "hant"):
		return ZH
	default:
		return EN
	}
}

func userLocale() string {
	// GetUserDefaultLocaleName writes a BCP-47 tag such as "tr-TR".
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")
	buf := make([]uint16, 85) // LOCALE_NAME_MAX_LENGTH
	n, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n <= 1 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n-1])
}

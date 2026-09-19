package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var mciSendString = syscall.NewLazyDLL("winmm.dll").NewProc("mciSendStringW")

func mci(cmd string) uintptr {
	p, err := syscall.UTF16PtrFromString(cmd)
	if err != nil {
		return 1
	}
	r, _, _ := mciSendString.Call(uintptr(unsafe.Pointer(p)), 0, 0, 0)
	return r
}

// playSound plays an audio file asynchronously through the Windows MCI API.
func playSound(path string) error {
	const alias = "poe2filter_preview"
	mci("close " + alias)
	if r := mci(fmt.Sprintf(`open "%s" type mpegvideo alias %s`, path, alias)); r != 0 {
		return fmt.Errorf("ses açılamadı (MCI %d)", r)
	}
	if r := mci("play " + alias); r != 0 {
		return fmt.Errorf("ses çalınamadı (MCI %d)", r)
	}
	return nil
}

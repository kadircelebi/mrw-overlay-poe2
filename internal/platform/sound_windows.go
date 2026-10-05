package platform

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

// PlaySound plays an audio file asynchronously through the Windows MCI API,
// at a filter volume (1..300, 0 for the loudest).
func PlaySound(path string, volume, maxVolume int) error {
	const alias = "poe2filter_preview"
	mci("close " + alias)
	if r := mci(fmt.Sprintf(`open "%s" type mpegvideo alias %s`, path, alias)); r != 0 {
		return fmt.Errorf("could not open the sound (MCI %d)", r)
	}
	if volume > 0 && volume < maxVolume {
		// MCI counts 0..1000; the filter's 300 is full volume.
		mci(fmt.Sprintf("setaudio %s volume to %d", alias, volume*1000/maxVolume))
	}
	if r := mci("play " + alias); r != 0 {
		return fmt.Errorf("could not play the sound (MCI %d)", r)
	}
	return nil
}

//go:build !windows

package overlay

import "image"

func ForegroundWindow() uintptr                  { return 0 }
func WindowPID(uintptr) uint32                   { return 0 }
func IsGameWindow(uintptr) bool                  { return false }
func GameCenter() (int, int, bool)               { return 0, 0, false }
func GameSize() (int, int, bool)                 { return 0, 0, false }
func GameClient() (image.Rectangle, bool)        { return image.Rectangle{}, false }
func WindowRect(uintptr) (image.Rectangle, bool) { return image.Rectangle{}, false }
func PlaceInGame(uintptr, bool)                  {}
func Confine(uintptr)                            {}
func FocusGame() (bool, bool)                    { return false, false }
func GameLogPath(uintptr) string                 { return "" }
func ShowOver(uintptr, image.Rectangle)          {}

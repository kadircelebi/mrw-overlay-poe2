//go:build !windows

package overlay

func ForegroundWindow() uintptr    { return 0 }
func WindowPID(uintptr) uint32     { return 0 }
func IsGameWindow(uintptr) bool    { return false }
func GameCenter() (int, int, bool) { return 0, 0, false }
func PlaceInGame(uintptr, bool)    {}
func Confine(uintptr)              {}
func FocusGame() (bool, bool)      { return false, false }

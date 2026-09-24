//go:build !windows

package overlay

import "errors"

func CopyAdvancedItem(bool) error {
	return errors.New("advanced item copy is only supported on Windows")
}
func CursorPosition() (int, int, bool) { return 0, 0, false }

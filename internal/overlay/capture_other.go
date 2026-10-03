//go:build !windows

package overlay

import "errors"

func CopyAdvancedItem(bool) error {
	return errors.New("advanced item copy is only supported on Windows")
}
func CursorPosition() (int, int, bool) { return 0, 0, false }
func ReadGameText() ([]OcrLine, int, int, error) {
	return nil, 0, 0, errors.New("screen text reading is only supported on Windows")
}
func PasteChatLine() {}

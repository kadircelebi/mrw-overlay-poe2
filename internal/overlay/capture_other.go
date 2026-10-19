//go:build !windows

package overlay

import (
	"errors"
	"image"
)

func CopyAdvancedItem(bool) error {
	return errors.New("advanced item copy is only supported on Windows")
}
func CursorPosition() (int, int, bool) { return 0, 0, false }
func CaptureGameClient() (*image.RGBA, image.Rectangle, error) {
	return nil, image.Rectangle{}, errors.New("screen reading is only supported on Windows")
}
func CaptureScreenRect(image.Rectangle) (*image.RGBA, error) {
	return nil, errors.New("screen reading is only supported on Windows")
}
func MoveCursor(int, int) bool { return false }
func MouseButtonDown() bool    { return false }
func ReadGameText() ([]OcrLine, int, int, error) {
	return nil, 0, 0, errors.New("screen text reading is only supported on Windows")
}
func PasteChatLine() {}
func ReadRunePanel([]string) ([]RuneRow, image.Rectangle, error) {
	return nil, image.Rectangle{}, errors.New("screen text reading is only supported on Windows")
}
func ClickOrEscape() bool { return false }

// SetOCRLanguage has no recognizer to set off Windows.
func SetOCRLanguage(string) {}

// OCRLanguageInstalled is false: there is no recognizer off Windows.
func OCRLanguageInstalled(string) bool { return false }

func LeftButtonDown() bool { return false }

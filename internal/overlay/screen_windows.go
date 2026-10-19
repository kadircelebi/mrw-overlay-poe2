//go:build windows

package overlay

import (
	"errors"
	"image"
)

var procSetCursorPos = user32.NewProc("SetCursorPos")

// CaptureGameClient copies the game's client area off the screen, and
// tells where it is (physical pixels).
func CaptureGameClient() (*image.RGBA, image.Rectangle, error) {
	hwnd := gameWindow()
	if hwnd == 0 {
		return nil, image.Rectangle{}, errors.New("game window not found")
	}
	area, ok := clientRect(hwnd)
	if !ok || area.width() <= 0 || area.height() <= 0 {
		return nil, image.Rectangle{}, errors.New("game window has no area")
	}
	pixels, err := captureScreen(area)
	if err != nil {
		return nil, image.Rectangle{}, err
	}
	w, h := int(area.width()), int(area.height())
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// The screen copy is BGRA; the picture wants RGBA.
	for i := 0; i+3 < len(pixels) && i+3 < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pixels[i+2], pixels[i+1], pixels[i], 255
	}
	return img, image.Rect(int(area.Left), int(area.Top), int(area.Right), int(area.Bottom)), nil
}

// MoveCursor puts the mouse pointer at screen pixel (x, y). It sends no
// click and no key: the game sees the pointer move, as when the player
// moves the mouse.
func MoveCursor(x, y int) bool {
	r, _, _ := procSetCursorPos.Call(uintptr(int32(x)), uintptr(int32(y)))
	return r != 0
}

// MouseButtonDown reports whether a mouse button is held down now.
func MouseButtonDown() bool { return keyDown(0x01) || keyDown(0x02) || keyDown(0x04) }

// CaptureScreenRect copies screen rectangle r (physical pixels) as RGBA.
func CaptureScreenRect(r image.Rectangle) (*image.RGBA, error) {
	if r.Empty() {
		return nil, errors.New("empty area")
	}
	pixels, err := captureScreen(rect{Left: int32(r.Min.X), Top: int32(r.Min.Y), Right: int32(r.Max.X), Bottom: int32(r.Max.Y)})
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for i := 0; i+3 < len(pixels) && i+3 < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pixels[i+2], pixels[i+1], pixels[i], 255
	}
	return img, nil
}

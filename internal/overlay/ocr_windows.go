//go:build windows

package overlay

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// Windows' own text recognizer (Windows.Media.Ocr) reads the tooltip of an
// item the game will not copy, such as a gem socketed in the Skills panel. It
// ships with Windows, so nothing extra is installed; it is reached through
// the bare WinRT ABI, the interface layouts below come from the SDK headers.

var (
	combase                    = syscall.NewLazyDLL("combase.dll")
	procRoInitialize           = combase.NewProc("RoInitialize")
	procRoUninitialize         = combase.NewProc("RoUninitialize")
	procRoGetActivationFactory = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString    = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString    = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRaw    = combase.NewProc("WindowsGetStringRawBuffer")

	gdi32                      = syscall.NewLazyDLL("gdi32.dll")
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
)

type guid struct {
	d1     uint32
	d2, d3 uint16
	d4     [8]byte
}

var (
	iidOcrEngineStatics      = guid{0x5bffa85a, 0x3384, 0x3540, [8]byte{0x99, 0x40, 0x69, 0x91, 0x20, 0xd4, 0x28, 0xa8}}
	iidSoftwareBitmapStatics = guid{0xdf0385db, 0x672f, 0x4a9d, [8]byte{0x80, 0x6e, 0xc2, 0x44, 0x2f, 0x34, 0x3e, 0x86}}
	iidCryptoBufferStatics   = guid{0x320b7e22, 0x3cb0, 0x4cdf, [8]byte{0x86, 0x63, 0x1d, 0x28, 0x91, 0x00, 0x65, 0xeb}}
	iidLanguageFactory       = guid{0x9b0252ac, 0x0c27, 0x44f8, [8]byte{0xb7, 0x92, 0x97, 0x93, 0xfb, 0x66, 0xc6, 0x3e}}
	iidAsyncInfo             = guid{0x00000036, 0x0000, 0x0000, [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	errOcrUnavailable        = errors.New("windows ocr is not available")
	recognizeTimeout         = 4 * time.Second
)

const (
	bitmapPixelFormatBgra8    = 87
	asyncStatusStarted        = 0
	asyncStatusCompleted      = 1
	roInitMultithreaded       = 1
	rpcEChangedMode           = 0x80010106
	srcCopy                   = 0x00CC0020
	dibRGBColors              = 0
	vtableQueryInterface      = 0
	vtableRelease             = 2
	vtableVectorGetAt         = 6
	vtableVectorSize          = 7
	vtableAsyncGetResults     = 8
	vtableAsyncInfoStatus     = 7
	vtableStaticsTryCreate    = 9
	vtableStaticsUserProfile  = 10
	vtableEngineRecognize     = 6
	vtableResultLines         = 6
	vtableResultTextAngle     = 7
	vtableReferenceValue      = 6
	vtableLineWords           = 6
	vtableLineText            = 7
	vtableWordRect            = 6
	vtableWordText            = 7
	vtableBitmapFromBuffer    = 9
	vtableBufferFromByteArray = 9
	vtableLanguageCreate      = 6
)

// comObject is a WinRT interface pointer: its first field is the vtable.
type comObject struct{ vtbl *[64]uintptr }

func (o *comObject) call(index int, args ...uintptr) error {
	all := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	r, _, _ := syscall.SyscallN(o.vtbl[index], all...)
	if int32(r) < 0 {
		return fmt.Errorf("winrt call %d: 0x%08x", index, uint32(r))
	}
	return nil
}

func (o *comObject) release() {
	if o != nil {
		_ = o.call(vtableRelease)
	}
}

type hstring uintptr

func newHString(s string) (hstring, error) {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	var h hstring
	r, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	if int32(r) < 0 {
		return 0, fmt.Errorf("WindowsCreateString: 0x%08x", uint32(r))
	}
	return h, nil
}

func (h hstring) String() string {
	if h == 0 {
		return ""
	}
	var n uint32
	p, _, _ := procWindowsGetStringRaw.Call(uintptr(h), uintptr(unsafe.Pointer(&n)))
	if p == 0 || n == 0 {
		return ""
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(p)), n))
}

func (h hstring) free() {
	if h != 0 {
		procWindowsDeleteString.Call(uintptr(h))
	}
}

func activationFactory(class string, iid *guid) (*comObject, error) {
	name, err := newHString(class)
	if err != nil {
		return nil, err
	}
	defer name.free()
	var f *comObject
	r, _, _ := procRoGetActivationFactory.Call(uintptr(name), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&f)))
	if int32(r) < 0 || f == nil {
		return nil, fmt.Errorf("%s: 0x%08x", class, uint32(r))
	}
	return f, nil
}

// ReadGameText captures the game window and returns the text lines Windows
// recognizes on it, in screen pixels, with the cursor position.
func ReadGameText() ([]OcrLine, int, int, error) {
	hwnd := gameWindow()
	if hwnd == 0 {
		return nil, 0, 0, errors.New("game window not found")
	}
	area, ok := clientRect(hwnd)
	if !ok || area.width() <= 0 || area.height() <= 0 {
		return nil, 0, 0, errors.New("game window has no area")
	}
	pixels, err := captureScreen(area)
	if err != nil {
		return nil, 0, 0, err
	}
	lines, err := recognize(pixels, int(area.width()), int(area.height()))
	if err != nil {
		return nil, 0, 0, err
	}
	for i := range lines {
		lines[i].X += float64(area.Left)
		lines[i].Y += float64(area.Top)
	}
	cx, cy, _ := CursorPosition()
	return lines, cx, cy, nil
}

type bitmapInfo struct {
	size                     uint32
	width, height            int32
	planes, bitCount         uint16
	compression, sizeImage   uint32
	xPerMeter, yPerMeter     int32
	colorsUsed, colorsImport uint32
	colors                   [1]uint32
}

// captureScreen copies the screen area as top-down BGRA with opaque alpha.
func captureScreen(area rect) ([]byte, error) {
	w, h := area.width(), area.height()
	screen, _, _ := procGetDC.Call(0)
	if screen == 0 {
		return nil, errors.New("GetDC failed")
	}
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	if mem == 0 {
		return nil, errors.New("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(mem)
	bmp, _, _ := procCreateCompatibleBitmap.Call(screen, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, errors.New("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelectObject.Call(mem, bmp)
	ok, _, _ := procBitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), screen, uintptr(area.Left), uintptr(area.Top), srcCopy)
	procSelectObject.Call(mem, old)
	if ok == 0 {
		return nil, errors.New("BitBlt failed")
	}
	info := bitmapInfo{width: w, height: -h, planes: 1, bitCount: 32}
	info.size = uint32(unsafe.Offsetof(info.colors))
	pixels := make([]byte, int(w)*int(h)*4)
	rows, _, _ := procGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&info)), dibRGBColors)
	if int32(rows) != h {
		return nil, errors.New("GetDIBits failed")
	}
	for i := 3; i < len(pixels); i += 4 {
		pixels[i] = 255
	}
	return pixels, nil
}

// recognize runs the recognizer on a BGRA image. English is preferred, item
// names being English; without that language pack the user's own is used.
func recognize(pixels []byte, w, h int) ([]OcrLine, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r, _, _ := procRoInitialize.Call(roInitMultithreaded)
	if int32(r) >= 0 {
		defer procRoUninitialize.Call()
	} else if uint32(r) != rpcEChangedMode {
		return nil, fmt.Errorf("RoInitialize: 0x%08x", uint32(r))
	}

	engine, err := ocrEngine()
	if err != nil {
		return nil, err
	}
	defer engine.release()

	crypto, err := activationFactory("Windows.Security.Cryptography.CryptographicBuffer", &iidCryptoBufferStatics)
	if err != nil {
		return nil, err
	}
	defer crypto.release()
	var buffer *comObject
	if err := crypto.call(vtableBufferFromByteArray, uintptr(len(pixels)), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&buffer))); err != nil {
		return nil, err
	}
	defer buffer.release()

	bitmaps, err := activationFactory("Windows.Graphics.Imaging.SoftwareBitmap", &iidSoftwareBitmapStatics)
	if err != nil {
		return nil, err
	}
	defer bitmaps.release()
	var bitmap *comObject
	if err := bitmaps.call(vtableBitmapFromBuffer, uintptr(unsafe.Pointer(buffer)), bitmapPixelFormatBgra8, uintptr(w), uintptr(h), uintptr(unsafe.Pointer(&bitmap))); err != nil {
		return nil, err
	}
	defer bitmap.release()

	var op *comObject
	if err := engine.call(vtableEngineRecognize, uintptr(unsafe.Pointer(bitmap)), uintptr(unsafe.Pointer(&op))); err != nil {
		return nil, err
	}
	defer op.release()
	if err := await(op); err != nil {
		return nil, err
	}
	var result *comObject
	if err := op.call(vtableAsyncGetResults, uintptr(unsafe.Pointer(&result))); err != nil {
		return nil, err
	}
	defer result.release()
	lines, err := resultLines(result)
	if err != nil {
		return nil, err
	}
	if angle := textAngle(result); angle != 0 {
		unrotate(lines, angle, float64(w)/2, float64(h)/2)
	}
	return lines, nil
}

// textAngle is the slant the recognizer found in the text and straightened
// before reading, in degrees clockwise; 0 when none.
func textAngle(result *comObject) float64 {
	var ref *comObject
	if result.call(vtableResultTextAngle, uintptr(unsafe.Pointer(&ref))) != nil || ref == nil {
		return 0
	}
	defer ref.release()
	var v float64
	if ref.call(vtableReferenceValue, uintptr(unsafe.Pointer(&v))) != nil {
		return 0
	}
	return v
}

func ocrEngine() (*comObject, error) {
	statics, err := activationFactory("Windows.Media.Ocr.OcrEngine", &iidOcrEngineStatics)
	if err != nil {
		return nil, err
	}
	defer statics.release()
	var engine *comObject
	if languages, err := activationFactory("Windows.Globalization.Language", &iidLanguageFactory); err == nil {
		tag, _ := newHString("en-US")
		var english *comObject
		if languages.call(vtableLanguageCreate, uintptr(tag), uintptr(unsafe.Pointer(&english))) == nil && english != nil {
			_ = statics.call(vtableStaticsTryCreate, uintptr(unsafe.Pointer(english)), uintptr(unsafe.Pointer(&engine)))
			english.release()
		}
		tag.free()
		languages.release()
	}
	if engine == nil {
		_ = statics.call(vtableStaticsUserProfile, uintptr(unsafe.Pointer(&engine)))
	}
	if engine == nil {
		return nil, errOcrUnavailable
	}
	return engine, nil
}

func await(op *comObject) error {
	var info *comObject
	if err := op.call(vtableQueryInterface, uintptr(unsafe.Pointer(&iidAsyncInfo)), uintptr(unsafe.Pointer(&info))); err != nil {
		return err
	}
	defer info.release()
	deadline := time.Now().Add(recognizeTimeout)
	for {
		var status uint32
		if err := info.call(vtableAsyncInfoStatus, uintptr(unsafe.Pointer(&status))); err != nil {
			return err
		}
		switch {
		case status == asyncStatusCompleted:
			return nil
		case status != asyncStatusStarted:
			return fmt.Errorf("ocr ended with status %d", status)
		case time.Now().After(deadline):
			return errors.New("ocr timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type winrtRect struct{ x, y, w, h float32 }

func resultLines(result *comObject) ([]OcrLine, error) {
	var lines *comObject
	if err := result.call(vtableResultLines, uintptr(unsafe.Pointer(&lines))); err != nil {
		return nil, err
	}
	defer lines.release()
	var n uint32
	if err := lines.call(vtableVectorSize, uintptr(unsafe.Pointer(&n))); err != nil {
		return nil, err
	}
	out := make([]OcrLine, 0, n)
	for i := uint32(0); i < n; i++ {
		var line *comObject
		if lines.call(vtableVectorGetAt, uintptr(i), uintptr(unsafe.Pointer(&line))) != nil || line == nil {
			continue
		}
		var text hstring
		_ = line.call(vtableLineText, uintptr(unsafe.Pointer(&text)))
		entry := OcrLine{Text: text.String()}
		text.free()
		entry.X, entry.Y, entry.W, entry.H, entry.TextRight = lineBox(line)
		line.release()
		out = append(out, entry)
	}
	return out, nil
}

// lineBox is the union of the line's word boxes.
// lineBox is the box around a line's words, and the right edge of its last
// word of two letters or digits (a scrollbar's edge can come out as a
// trailing ")" or "I").
func lineBox(line *comObject) (x, y, w, h, textRight float64) {
	var words *comObject
	if line.call(vtableLineWords, uintptr(unsafe.Pointer(&words))) != nil || words == nil {
		return
	}
	defer words.release()
	var n uint32
	_ = words.call(vtableVectorSize, uintptr(unsafe.Pointer(&n)))
	first := true
	var x2, y2 float64
	for i := uint32(0); i < n; i++ {
		var word *comObject
		if words.call(vtableVectorGetAt, uintptr(i), uintptr(unsafe.Pointer(&word))) != nil || word == nil {
			continue
		}
		var r winrtRect
		err := word.call(vtableWordRect, uintptr(unsafe.Pointer(&r)))
		var text hstring
		if err == nil && word.call(vtableWordText, uintptr(unsafe.Pointer(&text))) == nil {
			if isRealWord(text.String()) {
				textRight = max(textRight, float64(r.x+r.w))
			}
			text.free()
		}
		word.release()
		if err != nil {
			continue
		}
		left, top, right, bottom := float64(r.x), float64(r.y), float64(r.x+r.w), float64(r.y+r.h)
		if first {
			x, y, x2, y2, first = left, top, right, bottom, false
			continue
		}
		x, y = min(x, left), min(y, top)
		x2, y2 = max(x2, right), max(y2, bottom)
	}
	if textRight == 0 {
		textRight = x2
	}
	return x, y, x2 - x, y2 - y, textRight
}

// runePanelWidths are the shares of the game's width read for the Runeshape
// panel, which keeps to the left (at 4K its rows end at 28%). The recognizer
// is not steady: it now and then returns nothing at all for an image it
// reads at a slightly different size, or drops a row, so both widths are
// read and their rows merged.
var runePanelWidths = []float64{0.5, 0.7}

// ReadRunePanel captures the left of the game window once and returns the
// rows of Expedition's Runeshape Combinations panel, in pixels of the game's
// client area, with that area's place on the screen. No panel on screen is
// no error: the rows come back empty.
func ReadRunePanel(currencyNames []string) ([]RuneRow, image.Rectangle, error) {
	hwnd := gameWindow()
	if hwnd == 0 {
		return nil, image.Rectangle{}, errors.New("game window not found")
	}
	area, ok := clientRect(hwnd)
	if !ok || area.width() <= 0 || area.height() <= 0 {
		return nil, image.Rectangle{}, errors.New("game window has no area")
	}
	client := image.Rect(int(area.Left), int(area.Top), int(area.Right), int(area.Bottom))
	w, h := int(area.width()), int(area.height())
	captured := int(float64(w) * runePanelWidths[len(runePanelWidths)-1])
	shot := area
	shot.Right = shot.Left + int32(captured)
	pixels, err := captureScreen(shot)
	if err != nil {
		return nil, client, err
	}
	rows, err := readRunePanel(pixels, captured, w, h, currencyNames)
	return rows, client, err
}

// readRunePanel reads the panel off the captured left part of the game
// (captured pixels wide; w×h is the whole client area).
func readRunePanel(pixels []byte, captured, w, h int, currencyNames []string) ([]RuneRow, error) {
	names := nameKeys(currencyNames)
	var rows []RuneRow
	for _, share := range runePanelWidths {
		width := min(captured, int(float64(w)*share))
		lines, err := recognize(cropPixels(pixels, captured, 0, 0, width, h), width, h)
		if err != nil {
			return nil, err
		}
		if found, ok := FindRunePanel(lines, currencyNames); ok {
			rows = mergeRunePanels(rows, found)
		}
	}
	for i := range rows {
		if rows[i].CountRead && rows[i].Name != "" {
			continue
		}
		for _, b := range recountBoxes(rows[i], rows) {
			x0, y0, x1, y1 := b[0], b[1], min(b[2], captured), min(b[3], h)
			if x1-x0 < 8 || y1-y0 < 8 {
				continue
			}
			if lines, err := recognize(cropPixels(pixels, captured, x0, y0, x1, y1), x1-x0, y1-y0); err == nil {
				if recountFrom(&rows[i], lines, names); rows[i].CountRead && rows[i].Name != "" {
					break
				}
			}
		}
	}
	return rows, nil
}

// cropPixels copies a rectangle out of top-down BGRA pixels of the given
// width.
func cropPixels(pixels []byte, width, x0, y0, x1, y1 int) []byte {
	if x0 == 0 && y0 == 0 && x1 == width && y1*width*4 == len(pixels) {
		return pixels
	}
	out := make([]byte, 0, (x1-x0)*(y1-y0)*4)
	for y := y0; y < y1; y++ {
		out = append(out, pixels[(y*width+x0)*4:(y*width+x1)*4]...)
	}
	return out
}

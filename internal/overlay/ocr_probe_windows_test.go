//go:build windows

package overlay

import (
	"os"
	"testing"
)

// TestOCRLanguagesInstalled lists which game languages this Windows can read
// off the screen (POE2_OCR_PROBE=1; it depends on the machine's packs).
func TestOCRLanguagesInstalled(t *testing.T) {
	if os.Getenv("POE2_OCR_PROBE") == "" {
		t.Skip("POE2_OCR_PROBE not set")
	}
	for lang, tag := range OCRTags {
		t.Logf("%s %s: %v", lang, tag, OCRLanguageInstalled(tag))
	}
}

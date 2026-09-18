// Package neversink downloads NeverSink's PoE2 filter (MIT licensed) and
// extracts the data we need from it.
package neversink

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"poe2filter/internal/prices"
)

const rawBaseURL = "https://raw.githubusercontent.com/NeverSinkDev/NeverSink-Filter-for-PoE2/main/"

// Presets are NeverSink's strictness levels, indexed by strictness.
var Presets = []string{
	"0-SOFT", "1-REGULAR", "2-SEMI-STRICT", "3-STRICT",
	"4-VERY-STRICT", "5-UBER-STRICT", "6-UBER-PLUS-STRICT",
}

// FileName returns the upstream file name of a preset.
func FileName(strictness int) (string, error) {
	if strictness < 0 || strictness >= len(Presets) {
		return "", fmt.Errorf("geçersiz strictness %d", strictness)
	}
	return fmt.Sprintf("NeverSink's filter 2 - %s.filter", Presets[strictness]), nil
}

// Ensure returns a local copy of the preset, downloading it when the cached
// copy is missing or older than maxAge. A stale copy is used if GitHub fails.
func Ensure(ctx context.Context, strictness int, dir string, maxAge time.Duration, userAgent string) (string, error) {
	name, err := FileName(strictness)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) < maxAge {
		return path, nil
	}

	dlErr := download(ctx, rawBaseURL+url.PathEscape(name), path, userAgent)
	if dlErr == nil {
		return path, nil
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil // stale but usable
	}
	return "", fmt.Errorf("NeverSink filtresi indirilemedi: %w", dlErr)
}

func download(ctx context.Context, u, dest, userAgent string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "NeverSink") || len(data) < 10000 {
		return fmt.Errorf("beklenmeyen içerik")
	}
	return prices.WriteFileAtomic(dest, data)
}

var quoted = regexp.MustCompile(`"([^"]+)"`)

// BaseTypes returns every BaseType named in the filter (lowercase -> canonical).
// These are the names the game accepts, so generated rules stick to them.
func BaseTypes(content string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "BaseType") {
			continue
		}
		for _, m := range quoted.FindAllStringSubmatch(line, -1) {
			out[strings.ToLower(m[1])] = m[1]
		}
	}
	return out
}

// ExceptionalBases returns the bases NeverSink ranks in its exceptional tiers
// (blocks tagged "$type->exotic->exceptional"). They are scanned first.
func ExceptionalBases(content string) map[string]bool {
	out := map[string]bool{}
	in := false
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "Show") || strings.HasPrefix(line, "Hide") || strings.HasPrefix(line, "Minimal") {
			in = strings.Contains(line, "$type->exotic->exceptional")
			continue
		}
		if in && strings.HasPrefix(trimmed, "BaseType") {
			for _, m := range quoted.FindAllStringSubmatch(trimmed, -1) {
				out[m[1]] = true
			}
		}
	}
	return out
}

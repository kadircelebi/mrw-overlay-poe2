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

	"poe2filter/internal/i18n"
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
		return "", fmt.Errorf("invalid strictness %d", strictness)
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
	return "", fmt.Errorf(i18n.T("err.neversinkDownload"), dlErr)
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
		return fmt.Errorf("unexpected content")
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

// Style is one of NeverSink's named looks (the "!tag" on a rule), e.g.
// "apex_stier" or "currency_a". Colours are "R G B [A]" as in the filter.
type Style struct {
	Tag       string
	Category  string // part before the first "_", e.g. "currency"
	Name      string // the rest, e.g. "a"
	Count     int    // rules using the style
	Bg        string
	Text      string
	Border    string
	Effect    string // PlayEffect, e.g. "Red" or "Purple Temp"
	IconColor string
	IconShape string
}

var styleTag = regexp.MustCompile(`!([a-z0-9]+_[a-z0-9_]+)\s*$`)

// Styles returns NeverSink's named styles in file order. Only Show rules with
// a background or text colour are taken; the first rule defines a style and
// later ones only add to its count.
func Styles(content string) []Style {
	var out []Style
	index := map[string]int{}
	var cur *Style
	flush := func() {
		if cur == nil {
			return
		}
		if i, seen := index[cur.Tag]; seen {
			out[i].Count++
		} else if cur.Bg != "" || cur.Text != "" {
			cur.Count = 1
			index[cur.Tag] = len(out)
			out = append(out, *cur)
		}
		cur = nil
	}
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "Show") || strings.HasPrefix(line, "Hide") || strings.HasPrefix(line, "Minimal") {
			flush()
			if m := styleTag.FindStringSubmatch(line); m != nil && strings.HasPrefix(line, "Show") {
				cat, name, _ := strings.Cut(m[1], "_")
				cur = &Style{Tag: m[1], Category: cat, Name: name}
			}
			continue
		}
		if cur == nil {
			continue
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			flush()
			continue
		}
		args := strings.Join(f[1:], " ")
		switch f[0] {
		case "SetBackgroundColor":
			cur.Bg = args
		case "SetTextColor":
			cur.Text = args
		case "SetBorderColor":
			cur.Border = args
		case "PlayEffect":
			cur.Effect = args
		case "MinimapIcon":
			if len(f) == 4 {
				cur.IconColor, cur.IconShape = f[2], f[3]
			}
		}
	}
	flush()
	return out
}

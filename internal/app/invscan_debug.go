package app

import (
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"poe2filter/internal/invscan"
	"poe2filter/internal/lootlog"
)

// The cells a read could not read for sure are kept as pictures in
// <data>/debug/invscan, named by when, where and why, to look into later;
// a cell is kept again only once what was read of it changes, and only the
// newest doubtPictures stay.
const doubtPictures = 60

func (s *AppService) keepDoubtPictures(pic invscan.Img, g invscan.Grid, doubts []lootlog.Doubt) {
	if s.meta.DataDir == "" {
		return
	}
	keys := map[string]bool{}
	var fresh []lootlog.Doubt
	s.invScan.mu.Lock()
	for _, d := range doubts {
		key := fmt.Sprintf("%d,%d,%s,%d,%s", d.Row, d.Col, d.Name, d.Count, d.Why)
		keys[key] = true
		if !s.invScan.doubtsKept[key] {
			fresh = append(fresh, d)
		}
	}
	s.invScan.doubtsKept = keys
	s.invScan.mu.Unlock()
	if len(fresh) == 0 {
		return
	}
	dir := filepath.Join(s.meta.DataDir, "debug", "invscan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("inventory scan: debug pictures: %v", err)
		return
	}
	stamp := time.Now().Format("20060102-150405")
	pad := invscan.Pad(g.Cell)
	for _, d := range fresh {
		name := fmt.Sprintf("%s_r%dc%d_%s_%dx_%s.png", stamp, d.Row, d.Col, d.Why, d.Count, fileSafe(d.Name))
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			log.Printf("inventory scan: debug pictures: %v", err)
			return
		}
		err = png.Encode(f, pic.Picture(g.Padded(d.Col-1, d.Row-1, pad)))
		f.Close()
		if err != nil {
			log.Printf("inventory scan: debug pictures: %v", err)
		}
	}
	pruneDir(dir, doubtPictures)
}

func fileSafe(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '_'
	}, name)
}

// pruneDir keeps the newest keep files of dir (by name: they begin with
// their time).
func pruneDir(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= keep {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	for _, n := range names[:max(0, len(names)-keep)] {
		_ = os.Remove(filepath.Join(dir, n))
	}
}

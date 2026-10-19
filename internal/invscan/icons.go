package invscan

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"poe2filter/internal/useragent"
)

// Source is an item and the address of its icon.
type Source struct {
	Entry
	URL string
}

// IconCache downloads icons once into Dir and reads them from there after.
// The trade site serves its exchange items' icons as PNG; an icon that
// cannot be had is left out, and the others still load.
type IconCache struct {
	Dir    string
	Client *http.Client
	// Workers is how many downloads run at once (2 when zero), Pause the
	// rest between two downloads of one worker: the first load fetches a
	// few hundred small files from GGG's image server.
	Workers int
	Pause   time.Duration
}

// maxIconBytes caps one download; the trade site's icons are a few kB.
const maxIconBytes = 1 << 20

// Load returns one Icon per distinct address, with every source that uses
// it, in the order the addresses first appear. err tells how many could
// not be loaded (the rest are returned all the same).
func (c IconCache) Load(ctx context.Context, sources []Source) ([]Icon, error) {
	var order []string
	byURL := map[string][]Entry{}
	for _, s := range sources {
		if s.URL == "" {
			continue
		}
		if _, ok := byURL[s.URL]; !ok {
			order = append(order, s.URL)
		}
		byURL[s.URL] = append(byURL[s.URL], s.Entry)
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return nil, err
	}
	icons := make([]Icon, len(order))
	errs := make([]error, len(order))
	workers := c.Workers
	if workers <= 0 {
		workers = 2
	}
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				u := order[i]
				img, fetched, err := c.icon(ctx, u)
				if err != nil {
					errs[i] = fmt.Errorf("%s: %w", u, err)
					continue
				}
				icons[i] = Icon{Key: iconKey(u), Entries: byURL[u], Image: img}
				if fetched && c.Pause > 0 {
					select {
					case <-ctx.Done():
					case <-time.After(c.Pause):
					}
				}
			}
		}()
	}
	for i := range order {
		if ctx.Err() != nil {
			break
		}
		next <- i
	}
	close(next)
	wg.Wait()

	out := icons[:0]
	failed := 0
	var first error
	for i, ic := range icons {
		if errs[i] != nil || ic.Image == nil {
			failed++
			if first == nil {
				first = errs[i]
				if first == nil {
					first = ctx.Err()
				}
			}
			continue
		}
		out = append(out, ic)
	}
	if failed > 0 {
		return out, fmt.Errorf("%d of %d icons not loaded, first: %w", failed, len(order), first)
	}
	return out, nil
}

// iconKey names an icon's file after its address.
func iconKey(u string) string {
	sum := sha1.Sum([]byte(u))
	return hex.EncodeToString(sum[:8])
}

// icon reads the icon from the cache, else downloads and keeps it;
// fetched tells a download.
func (c IconCache) icon(ctx context.Context, u string) (image.Image, bool, error) {
	path := filepath.Join(c.Dir, iconKey(u)+".png")
	if f, err := os.Open(path); err == nil {
		img, _, err := image.Decode(f)
		f.Close()
		if err == nil {
			return img, false, nil
		}
		// A broken file (an interrupted write) is fetched again.
	}
	data, err := c.download(ctx, u)
	if err != nil {
		return nil, true, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, true, fmt.Errorf("not a PNG: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err == nil {
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
		}
	}
	return img, true, nil
}

func (c IconCache) download(ctx context.Context, u string) ([]byte, error) {
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", useragent.Value())
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxIconBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxIconBytes {
		return nil, errors.New("icon too large")
	}
	return data, nil
}

// InScope tells the trade site's exchange sections whose items are read:
// all of them but waystones (their tier is not on the icon) and the
// unnamed rest.
func InScope(group string) bool {
	switch group {
	case "", "Waystones", "Misc":
		return false
	}
	return true
}

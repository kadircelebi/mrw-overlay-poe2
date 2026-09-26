// Package collector fetches market prices from upstream sources and builds a
// prices.Snapshot. It is used in-process by the desktop app today and is meant
// to run unchanged on a collector server later.
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"poe2filter/internal/useragent"
)

// Upstreams (GGG in particular) ask tools not to impersonate browsers.

// NewHTTPClient returns the client used for all upstream requests.
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

// getJSON performs a GET and decodes a JSON body, failing on non-2xx status.
func getJSON(ctx context.Context, c *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", useragent.Value())
	req.Header.Set("Accept", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("could not decode JSON (%s): %w", url, err)
	}
	return nil
}

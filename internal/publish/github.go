// Package publish uploads the scan servers' results to a GitHub release,
// where the app downloads them. Each file is an asset of one fixed release
// (tag "data"), replaced in place: the download URL never changes and the
// repository does not grow a commit per upload.
package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"poe2filter/internal/useragent"
)

// GitHub uploads release assets with a fine-grained token that may only
// write the data repository.
type GitHub struct {
	Repo  string // owner/name
	Tag   string // the release holding the files, e.g. "data"
	Token string

	API     string // https://api.github.com (tests override it)
	Uploads string // https://uploads.github.com
	HTTP    *http.Client
}

type release struct {
	ID     int64   `json:"id"`
	Assets []asset `json:"assets"`
}

type asset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (g *GitHub) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func (g *GitHub) base(def, set string) string {
	if set != "" {
		return strings.TrimRight(set, "/")
	}
	return def
}

func (g *GitHub) do(ctx context.Context, method, u, contentType string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", useragent.Value())
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := g.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := string(raw)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("github %s %s: HTTP %d: %s", method, strings.SplitN(u, "?", 2)[0], resp.StatusCode, msg)
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

var errNotFound = errors.New("not found")

// ensureRelease returns the data release, creating it on first use.
func (g *GitHub) ensureRelease(ctx context.Context) (release, error) {
	api := g.base("https://api.github.com", g.API)
	var rel release
	err := g.do(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/releases/tags/%s", api, g.Repo, url.PathEscape(g.Tag)), "", nil, &rel)
	if err == nil {
		return rel, nil
	}
	if !errors.Is(err, errNotFound) {
		return rel, err
	}
	body, _ := json.Marshal(map[string]any{
		"tag_name": g.Tag, "name": "Scan data",
		"body":        "Updated automatically by the scan servers. The app downloads these files; nothing here needs to be installed.",
		"make_latest": "true",
	})
	err = g.do(ctx, http.MethodPost, fmt.Sprintf("%s/repos/%s/releases", api, g.Repo), "application/json", body, &rel)
	return rel, err
}

// Put replaces one asset of the data release. The new file is uploaded under
// a temporary name first and renamed after the old one is gone, so the
// download URL is missing for a moment at most, never serves half a file.
func (g *GitHub) Put(ctx context.Context, name string, data []byte, contentType string) error {
	if g.Repo == "" || g.Tag == "" || g.Token == "" {
		return errors.New("publish: repo, tag and token are required")
	}
	rel, err := g.ensureRelease(ctx)
	if err != nil {
		return err
	}
	api := g.base("https://api.github.com", g.API)
	uploads := g.base("https://uploads.github.com", g.Uploads)
	temp := name + ".new"
	// A temporary file left by an interrupted upload is removed first.
	for _, a := range rel.Assets {
		if a.Name == temp {
			if err := g.do(ctx, http.MethodDelete, fmt.Sprintf("%s/repos/%s/releases/assets/%d", api, g.Repo, a.ID), "", nil, nil); err != nil {
				return err
			}
		}
	}
	var uploaded asset
	u := fmt.Sprintf("%s/repos/%s/releases/%d/assets?name=%s", uploads, g.Repo, rel.ID, url.QueryEscape(temp))
	if err := g.do(ctx, http.MethodPost, u, contentType, data, &uploaded); err != nil {
		return err
	}
	for _, a := range rel.Assets {
		if a.Name == name {
			if err := g.do(ctx, http.MethodDelete, fmt.Sprintf("%s/repos/%s/releases/assets/%d", api, g.Repo, a.ID), "", nil, nil); err != nil {
				return err
			}
		}
	}
	patch, _ := json.Marshal(map[string]string{"name": name})
	return g.do(ctx, http.MethodPatch, fmt.Sprintf("%s/repos/%s/releases/assets/%d", api, g.Repo, uploaded.ID), "application/json", patch, nil)
}

// Package profileclient talks to the public profile server
// (cmd/profilesrv). Every document it downloads goes through
// publicprofile.Decode, so the app trusts the server no more than the server
// trusts the app.
package profileclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/publicprofile"
	"poe2filter/internal/session"
	"poe2filter/internal/useragent"
)

// DefaultBase is the profile server.
const DefaultBase = "https://profiles.mrwproject.com"

// maxReply bounds what is read from the server.
const maxReply = publicprofile.MaxBytes + 64<<10

// Error is a refusal from the server; Code is its machine-readable reason
// ("profile_limit", "not_owner", "rate_limited"...).
type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string { return fmt.Sprintf("profile server: %s (HTTP %d)", e.Code, e.Status) }

// IsCode reports whether err is a server refusal with this code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Client is safe for concurrent use.
type Client struct {
	Base    string
	HTTP    *http.Client
	keyPath string

	mu  sync.Mutex
	key string
}

// New keeps the install key under dataDir.
func New(dataDir string) *Client {
	return &Client{
		Base:    DefaultBase,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
		keyPath: filepath.Join(dataDir, "profile-install.key"),
	}
}

// installKey returns this install's key, asking the server for one the
// first time. The key is what owns the profiles published from here.
func (c *Client) installKey(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.key != "" {
		return c.key, nil
	}
	if sealed, err := os.ReadFile(c.keyPath); err == nil {
		if plain, err := session.UnprotectSecret(sealed); err == nil && len(plain) > 0 {
			c.key = string(plain)
			return c.key, nil
		}
	}
	var out struct {
		Key string `json:"key"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/installs", "", nil, &out); err != nil {
		return "", err
	}
	if out.Key == "" {
		return "", errors.New("profile server sent no install key")
	}
	sealed, err := session.ProtectSecret([]byte(out.Key))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(c.keyPath, sealed, 0o600); err != nil {
		return "", err
	}
	c.key = out.Key
	return c.key, nil
}

// HasInstall reports whether this install already has a key (it has
// published or followed something before).
func (c *Client) HasInstall() bool {
	_, err := os.Stat(c.keyPath)
	return err == nil
}

func (c *Client) authed(ctx context.Context, method, path string, body, out any) error {
	key, err := c.installKey(ctx)
	if err != nil {
		return err
	}
	return c.call(ctx, method, path, key, body, out)
}

func (c *Client) call(ctx context.Context, method, path, key string, body, out any) error {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Base, "/")+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", useragent.Value())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReply))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = "http_error"
		}
		return &Error{Status: resp.StatusCode, Code: e.Error}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// List returns one page of published profiles, most followed first.
func (c *Client) List(ctx context.Context, query, tag string, page int) ([]publicprofile.Listing, error) {
	v := url.Values{}
	if query = strings.TrimSpace(query); query != "" {
		v.Set("q", query)
	}
	if tag != "" {
		v.Set("tag", tag)
	}
	if page > 0 {
		v.Set("page", fmt.Sprint(page))
	}
	path := "/v1/profiles"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out struct {
		Profiles []publicprofile.Listing `json:"profiles"`
	}
	err := c.call(ctx, http.MethodGet, path, "", nil, &out)
	return out.Profiles, err
}

// Get downloads a profile and checks its document.
func (c *Client) Get(ctx context.Context, id string) (publicprofile.Listing, publicprofile.Document, error) {
	var out struct {
		Profile  publicprofile.Listing `json:"profile"`
		Document json.RawMessage       `json:"document"`
	}
	if err := c.call(ctx, http.MethodGet, "/v1/profiles/"+url.PathEscape(id), "", nil, &out); err != nil {
		return publicprofile.Listing{}, publicprofile.Document{}, err
	}
	doc, err := publicprofile.Decode(out.Document)
	if err != nil {
		return publicprofile.Listing{}, publicprofile.Document{}, fmt.Errorf("profile %s: %w", id, err)
	}
	return out.Profile, doc, nil
}

// Publish creates a profile (id "") or replaces this install's own one.
func (c *Client) Publish(ctx context.Context, id, author string, meta publicprofile.Meta, doc publicprofile.Document) (publicprofile.Listing, error) {
	data, err := publicprofile.Encode(doc)
	if err != nil {
		return publicprofile.Listing{}, err
	}
	if meta, err = meta.Clean(); err != nil {
		return publicprofile.Listing{}, err
	}
	body := map[string]any{"author": author, "meta": meta, "document": json.RawMessage(data)}
	method, path := http.MethodPost, "/v1/profiles"
	if id != "" {
		method, path = http.MethodPut, "/v1/profiles/"+url.PathEscape(id)
	}
	var out struct {
		Profile publicprofile.Listing `json:"profile"`
	}
	err = c.authed(ctx, method, path, body, &out)
	return out.Profile, err
}

// Unpublish deletes this install's own profile.
func (c *Client) Unpublish(ctx context.Context, id string) error {
	return c.authed(ctx, http.MethodDelete, "/v1/profiles/"+url.PathEscape(id), nil, nil)
}

// Mine lists this install's own profiles (hidden ones included).
func (c *Client) Mine(ctx context.Context) ([]publicprofile.Listing, error) {
	if !c.HasInstall() {
		return nil, nil
	}
	var out struct {
		Profiles []publicprofile.Listing `json:"profiles"`
	}
	err := c.authed(ctx, http.MethodGet, "/v1/mine", nil, &out)
	return out.Profiles, err
}

// Versions returns the current version of each of ids; an id missing from
// the result was removed (deleted or hidden) on the server.
func (c *Client) Versions(ctx context.Context, ids []string) (map[string]int, error) {
	var out struct {
		Versions map[string]int `json:"versions"`
	}
	err := c.call(ctx, http.MethodPost, "/v1/versions", "", map[string]any{"ids": ids}, &out)
	return out.Versions, err
}

// Follow starts or stops following id and returns its follower count.
func (c *Client) Follow(ctx context.Context, id string, on bool) (int, error) {
	method := http.MethodPut
	if !on {
		method = http.MethodDelete
	}
	var out struct {
		Followers int `json:"followers"`
	}
	err := c.authed(ctx, method, "/v1/profiles/"+url.PathEscape(id)+"/follow", nil, &out)
	return out.Followers, err
}

// Report flags a profile for moderation.
func (c *Client) Report(ctx context.Context, id, reason, note string) error {
	return c.authed(ctx, http.MethodPost, "/v1/profiles/"+url.PathEscape(id)+"/report",
		map[string]string{"reason": reason, "note": strings.TrimSpace(note)}, nil)
}

package profileclient

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"poe2filter/internal/filter"
	"poe2filter/internal/profilesrv"
	"poe2filter/internal/publicprofile"
)

func newPair(t *testing.T) (*Client, *Client) {
	t.Helper()
	st, err := profilesrv.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer((&profilesrv.Server{Store: st, IPSalt: "t"}).Handler())
	t.Cleanup(srv.Close)
	a, b := New(t.TempDir()), New(t.TempDir())
	a.Base, b.Base = srv.URL, srv.URL
	return a, b
}

func doc(minValue float64) publicprofile.Document {
	c := filter.DefaultConfig()
	c.MinValue = minValue
	d, _ := publicprofile.FromConfig(c)
	return d
}

func TestPublishFollowUpdate(t *testing.T) {
	author, follower := newPair(t)
	ctx := t.Context()
	meta := publicprofile.Meta{Name: "Expedition", Tags: []string{"expedition"}}

	l, err := author.Publish(ctx, "", "MrWGambling#1234", meta, doc(5))
	if err != nil {
		t.Fatal(err)
	}
	if !author.HasInstall() || follower.HasInstall() {
		t.Fatal("install key should exist only after an authenticated call")
	}
	// A fresh client on the same folder reuses the sealed key.
	again := New(filepath.Dir(author.keyPath))
	again.Base = author.Base
	if mine, err := again.Mine(ctx); err != nil || len(mine) != 1 || mine[0].ID != l.ID {
		t.Fatalf("stored key not reused: %v %v", mine, err)
	}

	list, err := follower.List(ctx, "exped", "expedition", 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	got, d, err := follower.Get(ctx, l.ID)
	if err != nil || got.Author != "MrWGambling#1234" || d.Filter.MinValue != 5 {
		t.Fatalf("get: %+v %v", got, err)
	}
	if n, err := follower.Follow(ctx, l.ID, true); err != nil || n != 1 {
		t.Fatalf("follow: %d %v", n, err)
	}

	if _, err := follower.Publish(ctx, l.ID, "X#0001", meta, doc(1)); !IsCode(err, "not_owner") {
		t.Fatalf("follower overwrote the profile: %v", err)
	}
	if _, err := author.Publish(ctx, l.ID, "MrWGambling#1234", meta, doc(9)); err != nil {
		t.Fatal(err)
	}
	v, err := follower.Versions(ctx, []string{l.ID})
	if err != nil || v[l.ID] != 2 {
		t.Fatalf("versions: %v %v", v, err)
	}
	if err := follower.Report(ctx, l.ID, "spam", ""); err != nil {
		t.Fatal(err)
	}
	if err := author.Unpublish(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	if v, _ := follower.Versions(ctx, []string{l.ID}); len(v) != 0 {
		t.Fatal("unpublished profile still has a version")
	}
}

// The app checks what it downloads even if the server (or something in
// between) sends a document the app would never accept.
func TestClientRefusesBadDocumentFromServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"profile":{"id":"abcdefghijkl"},"document":{"format":1,"filter":{"custom_base_filter":"C:\\x"}}}`))
	}))
	defer srv.Close()
	c := New(t.TempDir())
	c.Base = srv.URL
	if _, _, err := c.Get(t.Context(), "abcdefghijkl"); err == nil {
		t.Fatal("tampered document accepted")
	}
}

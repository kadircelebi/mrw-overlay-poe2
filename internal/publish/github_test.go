package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeGitHub keeps one repository's releases and assets in memory.
type fakeGitHub struct {
	mu       sync.Mutex
	release  *release
	contents map[int64][]byte
	nextID   int64
	calls    []string
}

func (f *fakeGitHub) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("%s %s without the token", r.Method, r.URL.Path)
		}
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		f.nextID++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/me/data/releases/tags/data":
			if f.release == nil {
				http.NotFound(w, r)
				return
			}
			json.NewEncoder(w).Encode(f.release)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/me/data/releases":
			f.release = &release{ID: 7}
			json.NewEncoder(w).Encode(f.release)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/me/data/releases/7/assets":
			body, _ := io.ReadAll(r.Body)
			a := asset{ID: f.nextID, Name: r.URL.Query().Get("name")}
			f.release.Assets = append(f.release.Assets, a)
			f.contents[a.ID] = body
			json.NewEncoder(w).Encode(a)
		case strings.HasPrefix(r.URL.Path, "/repos/me/data/releases/assets/"):
			var id int64
			fmt.Sscan(strings.TrimPrefix(r.URL.Path, "/repos/me/data/releases/assets/"), &id)
			for i, a := range f.release.Assets {
				if a.ID != id {
					continue
				}
				if r.Method == http.MethodDelete {
					f.release.Assets = append(f.release.Assets[:i], f.release.Assets[i+1:]...)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				var patch struct{ Name string }
				json.NewDecoder(r.Body).Decode(&patch)
				f.release.Assets[i].Name = patch.Name
				json.NewEncoder(w).Encode(f.release.Assets[i])
				return
			}
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "no", 500)
		}
	})
}

func (f *fakeGitHub) files() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for _, a := range f.release.Assets {
		out[a.Name] = string(f.contents[a.ID])
	}
	return out
}

func TestPutCreatesTheReleaseAndReplacesTheFile(t *testing.T) {
	fake := &fakeGitHub{contents: map[int64][]byte{}}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()
	g := &GitHub{Repo: "me/data", Tag: "data", Token: "tok", API: server.URL, Uploads: server.URL}

	if err := g.Put(context.Background(), "exceptional-office.json.gz", []byte("one"), "application/gzip"); err != nil {
		t.Fatal(err)
	}
	if err := g.Put(context.Background(), "exceptional-home.json.gz", []byte("home"), "application/gzip"); err != nil {
		t.Fatal(err)
	}
	if err := g.Put(context.Background(), "exceptional-office.json.gz", []byte("two"), "application/gzip"); err != nil {
		t.Fatal(err)
	}
	got := fake.files()
	if len(got) != 2 || got["exceptional-office.json.gz"] != "two" || got["exceptional-home.json.gz"] != "home" {
		t.Fatalf("assets = %v", got)
	}
}

func TestPutClearsATemporaryFileLeftBehind(t *testing.T) {
	fake := &fakeGitHub{contents: map[int64][]byte{}, release: &release{ID: 7, Assets: []asset{{ID: 90, Name: "a.gz.new"}}}}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()
	g := &GitHub{Repo: "me/data", Tag: "data", Token: "tok", API: server.URL, Uploads: server.URL}
	if err := g.Put(context.Background(), "a.gz", []byte("x"), "application/gzip"); err != nil {
		t.Fatal(err)
	}
	if got := fake.files(); len(got) != 1 || got["a.gz"] != "x" {
		t.Fatalf("assets = %v", got)
	}
}

func TestPutNeedsAToken(t *testing.T) {
	if err := (&GitHub{Repo: "me/data", Tag: "data"}).Put(context.Background(), "a", nil, ""); err == nil {
		t.Fatal("no error without a token")
	}
}

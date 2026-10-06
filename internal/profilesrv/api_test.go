package profilesrv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"poe2filter/internal/filter"
	"poe2filter/internal/publicprofile"
)

type testServer struct {
	t     *testing.T
	h     http.Handler
	store *Store
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "profiles.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st, IPSalt: "test"}
	return &testServer{t: t, h: s.Handler(), store: st}
}

// do sends a request from addr ("" = 192.0.2.1) and decodes the reply.
func (ts *testServer) do(method, path, key, body, addr string) (int, map[string]any) {
	ts.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if addr != "" {
		r.RemoteAddr = addr
	}
	if key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (ts *testServer) install(addr string) string {
	ts.t.Helper()
	code, out := ts.do("POST", "/v1/installs", "", "", addr)
	if code != http.StatusCreated {
		ts.t.Fatalf("install: %d %v", code, out)
	}
	return out["key"].(string)
}

func documentJSON(t *testing.T, minValue float64) string {
	t.Helper()
	c := filter.DefaultConfig()
	c.MinValue = minValue
	c.Whitelist = []string{"Divine Orb"}
	doc, _ := publicprofile.FromConfig(c)
	data, err := publicprofile.Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func publishJSON(t *testing.T, name string, tags string, minValue float64) string {
	return fmt.Sprintf(`{"author":"MrWGambling#1234","meta":{"name":%q,"description":"","tags":[%s]},"document":%s}`,
		name, tags, documentJSON(t, minValue))
}

func TestInstallLimitPerAddress(t *testing.T) {
	ts := newTestServer(t)
	for i := 0; i < InstallsPerIPPerDay; i++ {
		ts.install("198.51.100.7:1000")
	}
	if code, _ := ts.do("POST", "/v1/installs", "", "", "198.51.100.7:1001"); code != http.StatusTooManyRequests {
		t.Fatalf("fourth install from one address: %d", code)
	}
	ts.install("198.51.100.8:1000")
}

func TestPublishFollowAndList(t *testing.T) {
	ts := newTestServer(t)
	author, a, b := ts.install("198.51.100.1:1"), ts.install("198.51.100.2:1"), ts.install("198.51.100.3:1")

	if code, _ := ts.do("POST", "/v1/profiles", "", publishJSON(t, "x", "", 1), ""); code != http.StatusUnauthorized {
		t.Fatalf("publish without key: %d", code)
	}
	code, out := ts.do("POST", "/v1/profiles", author, publishJSON(t, "Expedition farm", `"expedition"`, 1), "")
	if code != http.StatusCreated {
		t.Fatalf("publish: %d %v", code, out)
	}
	id := out["profile"].(map[string]any)["id"].(string)
	_, out = ts.do("POST", "/v1/profiles", author, publishJSON(t, "Leveling", `"leveling"`, 2), "")
	other := out["profile"].(map[string]any)["id"].(string)

	for _, k := range []string{a, b, a} {
		ts.do("PUT", "/v1/profiles/"+id+"/follow", k, "", "")
	}
	ts.do("PUT", "/v1/profiles/"+other+"/follow", a, "", "")
	_, out = ts.do("GET", "/v1/profiles", "", "", "")
	list := out["profiles"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["id"] != id || list[0].(map[string]any)["followers"].(float64) != 2 {
		t.Fatalf("list not ordered by followers: %v", list)
	}
	if _, out = ts.do("DELETE", "/v1/profiles/"+id+"/follow", b, "", ""); out["followers"].(float64) != 1 {
		t.Fatalf("unfollow count: %v", out)
	}

	_, out = ts.do("GET", "/v1/profiles?tag=leveling", "", "", "")
	if l := out["profiles"].([]any); len(l) != 1 || l[0].(map[string]any)["id"] != other {
		t.Fatalf("tag filter: %v", l)
	}
	_, out = ts.do("GET", "/v1/profiles?q=EXPED", "", "", "")
	if l := out["profiles"].([]any); len(l) != 1 || l[0].(map[string]any)["id"] != id {
		t.Fatalf("search: %v", l)
	}
	_, out = ts.do("GET", "/v1/profiles?q=mrwgambling", "", "", "")
	if l := out["profiles"].([]any); len(l) != 2 {
		t.Fatalf("author search: %v", l)
	}
	if code, _ := ts.do("GET", "/v1/profiles?tag=mine", "", "", ""); code != http.StatusBadRequest {
		t.Fatalf("unknown tag accepted: %d", code)
	}

	code, out = ts.do("GET", "/v1/profiles/"+id, "", "", "")
	if code != 200 {
		t.Fatalf("get: %d", code)
	}
	raw, _ := json.Marshal(out["document"])
	if _, err := publicprofile.Decode(raw); err != nil {
		t.Fatalf("served document does not decode: %v", err)
	}
}

func TestOnlyOwnerChangesAndVersions(t *testing.T) {
	ts := newTestServer(t)
	author, stranger := ts.install("198.51.100.1:1"), ts.install("198.51.100.2:1")
	_, out := ts.do("POST", "/v1/profiles", author, publishJSON(t, "Main", "", 1), "")
	id := out["profile"].(map[string]any)["id"].(string)

	if code, _ := ts.do("PUT", "/v1/profiles/"+id, stranger, publishJSON(t, "Stolen", "", 9), ""); code != http.StatusForbidden {
		t.Fatalf("stranger update: %d", code)
	}
	if code, _ := ts.do("DELETE", "/v1/profiles/"+id, stranger, "", ""); code != http.StatusForbidden {
		t.Fatalf("stranger delete: %d", code)
	}
	_, out = ts.do("PUT", "/v1/profiles/"+id, author, publishJSON(t, "Renamed", "", 1), "")
	if v := out["profile"].(map[string]any)["version"].(float64); v != 1 {
		t.Fatalf("rename alone changed the version: %v", v)
	}
	_, out = ts.do("PUT", "/v1/profiles/"+id, author, publishJSON(t, "Renamed", "", 5), "")
	if v := out["profile"].(map[string]any)["version"].(float64); v != 2 {
		t.Fatalf("document change kept the version: %v", v)
	}
	_, out = ts.do("POST", "/v1/versions", "", fmt.Sprintf(`{"ids":[%q,"zzzzzzzzzzz","../../etc"]}`, id), "")
	if v := out["versions"].(map[string]any); len(v) != 1 || v[id].(float64) != 2 {
		t.Fatalf("versions: %v", v)
	}

	if err := ts.store.SetHidden(t.Context(), id, true); err != nil {
		t.Fatal(err)
	}
	if code, _ := ts.do("GET", "/v1/profiles/"+id, "", "", ""); code != http.StatusNotFound {
		t.Fatalf("hidden profile served: %d", code)
	}
	_, out = ts.do("POST", "/v1/versions", "", fmt.Sprintf(`{"ids":[%q]}`, id), "")
	if len(out["versions"].(map[string]any)) != 0 {
		t.Fatal("hidden profile reported a version")
	}
	_, out = ts.do("GET", "/v1/mine", author, "", "")
	if l := out["profiles"].([]any); len(l) != 1 || l[0].(map[string]any)["hidden"] != true {
		t.Fatalf("owner does not see the hidden profile: %v", l)
	}
	if code, _ := ts.do("DELETE", "/v1/profiles/"+id, author, "", ""); code != http.StatusNoContent {
		t.Fatalf("owner delete: %d", code)
	}
}

func TestPublishRefusesBadInput(t *testing.T) {
	ts := newTestServer(t)
	key := ts.install("")
	doc := documentJSON(t, 1)
	bad := map[string]string{
		"script field":     `{"author":"A#1234","meta":{"name":"x","description":"","tags":[]},"document":` + doc + `,"script":"x"}`,
		"tampered doc":     `{"author":"A#1234","meta":{"name":"x","description":"","tags":[]},"document":{"format":1,"filter":{"price_source_url":"http://x"}}}`,
		"author no tag":    `{"author":"A","meta":{"name":"x","description":"","tags":[]},"document":` + doc + `}`,
		"author newline":   `{"author":"A\n#1234","meta":{"name":"x","description":"","tags":[]},"document":` + doc + `}`,
		"free tag":         `{"author":"A#1234","meta":{"name":"x","description":"","tags":["hello"]},"document":` + doc + `}`,
		"empty name":       `{"author":"A#1234","meta":{"name":"","description":"","tags":[]},"document":` + doc + `}`,
		"not json":         `<script>`,
		"two docs":         `{"author":"A#1234","meta":{"name":"x","description":"","tags":[]},"document":` + doc + `} {}`,
		"html description": `{"author":"A#1234","meta":{"name":"x","description":"<b>\nhi","tags":[]},"document":` + doc + `}`,
	}
	for name, body := range bad {
		if code, out := ts.do("POST", "/v1/profiles", key, body, ""); code != http.StatusBadRequest {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	big := `{"author":"A#1234","meta":{"name":"x","description":"","tags":[]},"document":"` + strings.Repeat("a", maxBody) + `"}`
	if code, _ := ts.do("POST", "/v1/profiles", key, big, ""); code != http.StatusBadRequest {
		t.Errorf("oversized body: %d", code)
	}
	if code, _ := ts.do("POST", "/v1/profiles", "not-a-key", publishJSON(t, "x", "", 1), ""); code != http.StatusUnauthorized {
		t.Errorf("malformed key: %d", code)
	}
}

func TestProfileLimitAndReports(t *testing.T) {
	ts := newTestServer(t)
	key, other := ts.install(""), ts.install("198.51.100.9:1")
	var id string
	for i := 0; i < MaxProfilesPerInstall; i++ {
		code, out := ts.do("POST", "/v1/profiles", key, publishJSON(t, fmt.Sprint("p", i), "", 1), "")
		if code != http.StatusCreated {
			t.Fatalf("publish %d: %d", i, code)
		}
		id = out["profile"].(map[string]any)["id"].(string)
	}
	if code, _ := ts.do("POST", "/v1/profiles", key, publishJSON(t, "one more", "", 1), ""); code != http.StatusConflict {
		t.Fatalf("over the limit: %d", code)
	}
	if code, _ := ts.do("POST", "/v1/profiles/"+id+"/report", other, `{"reason":"hack","note":""}`, ""); code != http.StatusBadRequest {
		t.Fatalf("unknown reason: %d", code)
	}
	if code, _ := ts.do("POST", "/v1/profiles/"+id+"/report", other, `{"reason":"spam","note":"same filter 20 times"}`, ""); code != http.StatusNoContent {
		t.Fatalf("report: %d", code)
	}
	rows, err := ts.store.Reports(t.Context())
	if err != nil || len(rows) != 1 || rows[0].Profile != id {
		t.Fatalf("reports: %v %v", rows, err)
	}
}

func TestClientIPTrustsCloudflareOnly(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("CF-Connecting-IP", "203.0.113.5")
	r.RemoteAddr = "198.51.100.1:443"
	if got := ClientIP(r); got != "198.51.100.1" {
		t.Fatalf("header trusted from a stranger: %s", got)
	}
	r.RemoteAddr = "172.68.1.2:443"
	if got := ClientIP(r); got != "203.0.113.5" {
		t.Fatalf("Cloudflare header ignored: %s", got)
	}
	r.RemoteAddr = "[2606:4700::1]:443"
	if got := ClientIP(r); got != "203.0.113.5" {
		t.Fatalf("Cloudflare IPv6 header ignored: %s", got)
	}
}

func TestBackupVerifies(t *testing.T) {
	ts := newTestServer(t)
	key := ts.install("")
	ts.do("POST", "/v1/profiles", key, publishJSON(t, "Backed up", "", 1), "")
	path := filepath.Join(t.TempDir(), "copy.db")
	if err := ts.store.Backup(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	stats, err := Verify(t.Context(), path)
	if err != nil || !strings.Contains(stats, "profiles 1") {
		t.Fatalf("verify: %q %v", stats, err)
	}
	if err := os.WriteFile(path, []byte("not a database at all, just text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(t.Context(), path); err == nil {
		t.Fatal("a broken file passed verification")
	}
}

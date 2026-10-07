package profileclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

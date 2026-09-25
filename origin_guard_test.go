package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuntimeRefusesOtherOrigins(t *testing.T) {
	reached := false
	h := sameOriginRuntime(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	cases := []struct {
		path, origin string
		allowed      bool
	}{
		{"/wails/runtime", "http://wails.localhost", true},
		{"/wails/runtime", "", true},
		{"/wails/runtime", "https://www.pathofexile.com", false},
		{"/wails/runtime", "http://wails.localhost.evil.test", false},
		{"/wails/runtime", "null", false},
		{"/index.html", "https://www.pathofexile.com", true},
	}
	for _, c := range cases {
		reached = false
		req := httptest.NewRequest(http.MethodPost, "http://wails.localhost"+c.path, nil)
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if reached != c.allowed {
			t.Errorf("%s from %q: reached=%v, want %v (status %d)", c.path, c.origin, reached, c.allowed, rec.Code)
		}
	}
}

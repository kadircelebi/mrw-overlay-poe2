package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLeagues(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"result sarmalı", `{"result":[{"id":"Rise of the Abyssal","text":"Rise of the Abyssal"},
			{"id":"HC Rise of the Abyssal"},{"id":"Standard"},{"id":"Standard"}]}`,
			[]string{"Rise of the Abyssal", "HC Rise of the Abyssal", "Standard"}},
		{"çıplak dizi", `[{"id":"Standard"},{"text":"Hardcore"}]`, []string{"Standard", "Hardcore"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			got, err := fetchLeaguesFrom(context.Background(), srv.Client(), srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}

func TestFetchLeaguesEmptyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	defer srv.Close()
	if _, err := fetchLeaguesFrom(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("boş liste hata olmalı")
	}
}

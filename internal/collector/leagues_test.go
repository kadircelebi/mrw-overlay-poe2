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

func TestAutoLeague(t *testing.T) {
	now := []string{"Forbidden Rites", "HC Forbidden Rites", "Runes of Aldur", "HC Runes of Aldur", "Standard", "Hardcore"}
	launch := append([]string{"Next League", "HC Next League"}, now...)
	ended := []string{"Runes of Aldur", "HC Runes of Aldur", "Standard", "Hardcore"}
	for _, tc := range []struct {
		name        string
		list, known []string
		prev, want  string
	}{
		{"ilk seçim listenin başı", now, nil, "", "Forbidden Rites"},
		{"aynı liste, değişmez", now, now, "Forbidden Rites", "Forbidden Rites"},
		{"lig ortası açılan event lig geçmez", now, []string{"Forbidden Rites", "HC Forbidden Rites", "Standard", "Hardcore"}, "Forbidden Rites", "Forbidden Rites"},
		{"bilinen event lig başa geçse de kalır", append([]string{"Runes of Aldur"}, now...), now, "Forbidden Rites", "Forbidden Rites"},
		{"yeni lig açılışı", launch, now, "Forbidden Rites", "Next League"},
		{"geçmiş yoksa yeni sayılmaz", launch, nil, "Forbidden Rites", "Forbidden Rites"},
		{"lig bitti, yenisi yok: eskisi kalır", ended, now, "Forbidden Rites", "Forbidden Rites"},
		{"geçmiş yok, lig bitmiş: listenin başı", append([]string{"Next League"}, ended...), nil, "Forbidden Rites", "Next League"},
		{"lig bitti, sonra yenisi geldi", append([]string{"Next League"}, ended...), ended, "Forbidden Rites", "Next League"},
		{"aday yok", []string{"Standard", "Hardcore", "HC Foo", "Foo SSF", "Ruthless"}, nil, "", ""},
		{"aday yok, önceki korunur", []string{"Standard"}, nil, "Forbidden Rites", "Forbidden Rites"},
		{"büyük/küçük harf", now, now, "forbidden rites", "forbidden rites"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AutoLeague(tc.list, tc.known, tc.prev); got != tc.want {
				t.Errorf("AutoLeague = %q, want %q", got, tc.want)
			}
		})
	}
}

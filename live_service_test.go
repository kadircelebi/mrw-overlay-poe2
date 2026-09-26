package main

import (
	"slices"
	"testing"
)

func TestLiveSearchesAreRemembered(t *testing.T) {
	s := &AppService{meta: Meta{DataDir: t.TempDir()}}
	s.rememberLive("a", true)
	s.rememberLive("b", true)
	s.rememberLive("a", true) // started twice: kept once
	if got := s.rememberedLive(); !slices.Equal(got, []string{"b", "a"}) {
		t.Fatalf("remembered %v", got)
	}
	s.rememberLive("b", false)
	if got := s.rememberedLive(); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("after stop %v", got)
	}
}

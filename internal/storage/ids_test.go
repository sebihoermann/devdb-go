package storage

import "testing"

func TestNewID(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 {
		t.Fatalf("len=%d want 32", len(id))
	}
	id2, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if id == id2 {
		t.Fatal("ids should differ")
	}
}

func TestResolveID(t *testing.T) {
	candidates := []string{"abc123def456", "abc789fed321"}
	got, err := ResolveID("abc123def456", candidates)
	if err != nil || got != "abc123def456" {
		t.Fatalf("full match: %q err=%v", got, err)
	}
	got, err = ResolveID("abc123", candidates)
	if err != nil || got != "abc123def456" {
		t.Fatalf("prefix match: %q err=%v", got, err)
	}
	_, err = ResolveID("", candidates)
	if err == nil {
		t.Fatal("expected empty prefix error")
	}
	_, err = ResolveID("zzz", candidates)
	if err == nil {
		t.Fatal("expected no match error")
	}
	_, err = ResolveID("abc", candidates)
	if err == nil {
		t.Fatal("expected ambiguous prefix error")
	}
}

package auth

import (
	"testing"
	"time"
)

func TestTokenRoundtrip(t *testing.T) {
	tk := NewTokenKey()
	tok, err := tk.Issue("user-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := tk.Verify(tok)
	if err != nil || sub != "user-1" {
		t.Fatalf("sub=%q err=%v", sub, err)
	}
}

func TestTokenExpiry(t *testing.T) {
	tk := NewTokenKey()
	tok, _ := tk.Issue("u", -time.Minute)
	if _, err := tk.Verify(tok); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestTokenTamperAndWrongKey(t *testing.T) {
	tk := NewTokenKey()
	tok, _ := tk.Issue("u", time.Hour)
	if _, err := tk.Verify(tok + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
	other := NewTokenKey()
	if _, err := other.Verify(tok); err == nil {
		t.Fatal("foreign key accepted")
	}
}

func TestKeyPersistence(t *testing.T) {
	dir := t.TempDir()
	a, err := LoadOrCreateTokenKey(dir + "/k")
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadOrCreateTokenKey(dir + "/k")
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := a.Issue("u", time.Hour)
	if _, err := b.Verify(tok); err != nil {
		t.Fatalf("reloaded key must verify: %v", err)
	}
}

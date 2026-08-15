package secret

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCryptStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewEncryptedFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("acct:1", []byte(`{"pat":"topsecret"}`)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("acct:1")
	if err != nil || !bytes.Contains(got, []byte("topsecret")) {
		t.Fatalf("get=%q err=%v", got, err)
	}
	if err := s.Delete("acct:1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("acct:1"); err != ErrNotFound {
		t.Fatalf("err=%v", err)
	}
}

// Bytes on disk must be ciphertext: the plaintext may never appear.
func TestCryptStoreEncryptsAtRest(t *testing.T) {
	dir := t.TempDir()
	s, err := NewEncryptedFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Set("acct:1", []byte("PLAINTEXT-MARKER-42"))
	if !anyFileExists(dir) {
		t.Fatal("no secret file written")
	}
	if anyContains(dir, []byte("PLAINTEXT-MARKER-42")) {
		t.Fatal("plaintext leaked to disk")
	}
}

func anyFileExists(dir string) bool { return anyContains(dir, nil) }

func anyContains(dir string, marker []byte) bool {
	entries, err := os.ReadDir(filepath.Join(dir, "secrets"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "secrets", e.Name()))
		if err != nil {
			continue
		}
		if marker == nil || bytes.Contains(b, marker) {
			return true
		}
	}
	return false
}

func TestKeyFileReusedAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	a, _ := NewEncryptedFileStore(dir)
	_ = a.Set("k", []byte("value"))
	b, _ := NewEncryptedFileStore(dir)
	got, err := b.Get("k")
	if err != nil || string(got) != "value" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

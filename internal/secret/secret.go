// Package secret stores account credentials. OS keychain first
// (macOS Keychain / Windows Credential Manager / libsecret), encrypted-file
// fallback elsewhere. Full implementation lands in M2 with the OAuth flow;
// M1 ships the interface so the storage schema is settled.
package secret

import "errors"

// ErrNotFound is returned when a secret reference does not exist.
var ErrNotFound = errors.New("secret not found")

// Store persists provider credentials, addressed by an opaque reference
// stable across restarts.
type Store interface {
	// Set stores data under ref, creating or replacing it.
	Set(ref string, data []byte) error
	Get(ref string) ([]byte, error)
	Delete(ref string) error
}

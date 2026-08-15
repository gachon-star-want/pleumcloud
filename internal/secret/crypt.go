// At-rest encryption for the file-based secret fallback: AES-256-GCM with
// a key file under the data directory (0600, separate from the database).
// Entries are marked "pc1." so future format changes stay detectable.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// NewEncryptedFileStore builds the encrypted file store, creating or
// reusing <dataDir>/secret.key.
func NewEncryptedFileStore(dataDir string) (Store, error) {
	keyPath := filepath.Join(dataDir, "secret.key")
	key := make([]byte, 32)
	if b, err := os.ReadFile(keyPath); err == nil && len(b) == 32 {
		copy(key, b)
	} else {
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, key, 0o600); err != nil {
			return nil, err
		}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &cryptStore{inner: &fileStore{dir: filepath.Join(dataDir, "secrets")}, aead: aead}, nil
}

type cryptStore struct {
	inner Store
	aead  cipher.AEAD

	mu sync.Mutex
}

func aesErr(err error) error { return fmt.Errorf("secret crypt: %w", err) }

func (c *cryptStore) Set(ref string, data []byte) error {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return aesErr(err)
	}
	sealed := c.aead.Seal(nil, nonce, data, nil)
	buf := append([]byte("pc1."), nonce...)
	buf = append(buf, sealed...)
	return c.inner.Set(ref, buf)
}

func (c *cryptStore) Get(ref string) ([]byte, error) {
	raw, err := c.inner.Get(ref)
	if err != nil {
		return nil, err
	}
	if len(raw) < 4 || string(raw[:4]) != "pc1." {
		// Legacy plaintext entry (pre-encryption database): pass through.
		return raw, nil
	}
	body := raw[4:]
	ns := c.aead.NonceSize()
	if len(body) < ns {
		return nil, aesErr(fmt.Errorf("entry too short"))
	}
	plain, err := c.aead.Open(nil, body[:ns], body[ns:], nil)
	if err != nil {
		return nil, aesErr(err)
	}
	return plain, nil
}

func (c *cryptStore) Delete(ref string) error { return c.inner.Delete(ref) }

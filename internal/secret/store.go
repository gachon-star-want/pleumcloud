// Package secret stores account credentials. OS keychain first
// (macOS Keychain / Windows Credential Manager / libsecret), with a
// permission-restricted file fallback for headless systems where no
// Secret Service exists.
package secret

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"
)

const keyringService = "pleumcloud"

// New returns the best available store for this system: the OS keychain
// when reachable, otherwise a 0600 file under dataDir/secrets.
func New(dataDir string) Store {
	k := &keyringStore{}
	if k.probe() {
		return k
	}
	return &fileStore{dir: filepath.Join(dataDir, "secrets")}
}

type keyringStore struct{}

func (k *keyringStore) probe() bool {
	const probeKey = "__probe__"
	if err := keyring.Set(keyringService, probeKey, "ok"); err != nil {
		return false
	}
	v, err := keyring.Get(keyringService, probeKey)
	_ = keyring.Delete(keyringService, probeKey)
	return err == nil && v == "ok"
}

func (k *keyringStore) Set(ref string, data []byte) error {
	return keyring.Set(keyringService, ref, string(data))
}

func (k *keyringStore) Get(ref string) ([]byte, error) {
	v, err := keyring.Get(keyringService, ref)
	if err != nil {
		if err == keyring.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return []byte(v), nil
}

func (k *keyringStore) Delete(ref string) error {
	err := keyring.Delete(keyringService, ref)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}

// fileStore keeps each secret as a 0600 JSON-wrapped file. It is the
// fallback of last resort; the keychain path is preferred wherever one
// exists.
type fileStore struct {
	dir string

	mu sync.Mutex
}

type fileEnvelope struct {
	Data []byte `json:"data"`
}

func (f *fileStore) path(ref string) string {
	safe := make([]byte, 0, len(ref))
	for _, c := range []byte(ref) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			safe = append(safe, c)
		} else {
			safe = append(safe, '_')
		}
	}
	return filepath.Join(f.dir, string(safe)+".json")
}

func (f *fileStore) Set(ref string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return fmt.Errorf("secrets dir: %w", err)
	}
	buf, err := json.Marshal(fileEnvelope{Data: data})
	if err != nil {
		return err
	}
	return os.WriteFile(f.path(ref), buf, 0o600)
}

func (f *fileStore) Get(ref string) ([]byte, error) {
	buf, err := os.ReadFile(f.path(ref))
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var env fileEnvelope
	if err := json.Unmarshal(buf, &env); err != nil {
		return nil, fmt.Errorf("corrupt secret %q: %w", ref, err)
	}
	return env.Data, nil
}

func (f *fileStore) Delete(ref string) error {
	err := os.Remove(f.path(ref))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// PutJSON marshals v and stores it under ref.
func PutJSON(s Store, ref string, v any) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Set(ref, buf)
}

// GetJSON reads and unmarshals the secret under ref.
func GetJSON(s Store, ref string, v any) error {
	buf, err := s.Get(ref)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf, v)
}

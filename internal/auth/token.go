// Bearer tokens: HMAC-SHA256-signed payloads (JWT-equivalent structure
// without the dependency). Tokens carry the user id and an expiry; the
// signing key lives in a 0600 file under the data directory.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type TokenKey struct{ key []byte }

func NewTokenKey() *TokenKey {
	k := make([]byte, 32)
	rand.Read(k)
	return &TokenKey{key: k}
}

// LoadOrCreateTokenKey reads (or creates, 0600) the signing key file.
func LoadOrCreateTokenKey(path string) (*TokenKey, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
		return &TokenKey{key: b}, nil
	}
	tk := NewTokenKey()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, tk.key, 0o600); err != nil {
		return nil, err
	}
	return tk, nil
}

type tokenPayload struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
}

// Issue signs a token for sub valid for ttl.
func (t *TokenKey) Issue(sub string, ttl time.Duration) (string, error) {
	payload, err := json.Marshal(tokenPayload{Sub: sub, Exp: time.Now().Add(ttl).Unix()})
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	sig := t.sign(body)
	return "v1." + body + "." + sig, nil
}

// Verify checks signature and expiry, returning the subject.
func (t *TokenKey) Verify(tok string) (string, error) {
	var body, sig string
	if n, err := fmt.Sscanf(tok, "v1.%s", &body); n != 1 || err != nil {
		return "", fmt.Errorf("malformed token")
	}
	if i := indexByte(body, '.'); i < 0 {
		return "", fmt.Errorf("malformed token")
	} else {
		sig = body[i+1:]
		body = body[:i]
	}
	if subtle.ConstantTimeCompare([]byte(sig), []byte(t.sign(body))) != 1 {
		return "", fmt.Errorf("bad signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", fmt.Errorf("bad payload")
	}
	var p tokenPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("bad payload json")
	}
	if time.Now().Unix() >= p.Exp {
		return "", fmt.Errorf("token expired")
	}
	return p.Sub, nil
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func (t *TokenKey) sign(body string) string {
	m := hmac.New(sha256.New, t.key)
	m.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

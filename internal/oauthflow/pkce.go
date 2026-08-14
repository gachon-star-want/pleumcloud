package oauthflow

import (
	"crypto/sha256"
	"encoding/base64"
)

// sha256Sum returns the S256 code challenge for a PKCE verifier.
func sha256Sum(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

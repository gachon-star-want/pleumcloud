// Package auth guards the HTTP surface in server mode: one shared
// password, constant-time compared, browser-native Basic auth (which also
// works for PWA installs). Multi-user accounts arrive with the hosted
// service.
package auth

import (
	"crypto/subtle"
	"net/http"
	"time"
)

// Guard wraps next with Basic-auth protection. Failures are delayed by
// failDelay to blunt brute force. In local mode (no password) servers
// stay unguarded and loopback-only.
func Guard(password string, failDelay time.Duration) func(http.Handler) http.Handler {
	want := []byte(password)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			userOK := subtle.ConstantTimeCompare([]byte(user), []byte("pleumcloud")) == 1
			passOK := ok && subtle.ConstantTimeCompare([]byte(pass), want) == 1
			if !userOK || !passOK {
				time.Sleep(failDelay)
				w.Header().Set("WWW-Authenticate", `Basic realm="PleumCloud", charset="UTF-8"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

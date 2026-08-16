package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostGuard(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	g := hostGuard(next)

	cases := []struct {
		host string
		want int
	}{
		{"127.0.0.1:7777", http.StatusOK},
		{"localhost:7777", http.StatusOK},
		{"[::1]:7777", http.StatusOK},
		{"localhost", http.StatusOK},
		{"127.0.0.1", http.StatusOK},
		{"evil.example", http.StatusMisdirectedRequest},
		{"evil.example:7777", http.StatusMisdirectedRequest},
		{"192.168.1.5:7777", http.StatusMisdirectedRequest},
		{"127.0.0.1.evil.example", http.StatusMisdirectedRequest},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "http://placeholder/", nil)
		req.Host = c.host
		rec := httptest.NewRecorder()
		g.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("Host %q = %d, want %d", c.host, rec.Code, c.want)
		}
	}
}

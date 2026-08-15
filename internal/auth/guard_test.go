package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGuardAllowsCorrectPassword(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := Guard("letmein", 0)(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("pleumcloud", "letmein")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestGuardRejectsWrongAndMissing(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := Guard("letmein", 0)(inner)

	for _, tc := range []struct {
		name, user, pass string
	}{
		{"missing", "", ""},
		{"wrong pass", "pleumcloud", "nope"},
		{"wrong user", "admin", "letmein"},
	} {
		req := httptest.NewRequest("GET", "/", nil)
		if tc.user != "" || tc.pass != "" {
			req.SetBasicAuth(tc.user, tc.pass)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Fatalf("%s: code = %d", tc.name, rec.Code)
		}
		if rec.Header().Get("WWW-Authenticate") == "" {
			t.Fatalf("%s: missing WWW-Authenticate", tc.name)
		}
	}
}

func TestGuardDelaysFailures(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := Guard("pw", 30*time.Millisecond)(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("pleumcloud", "bad")
	start := time.Now()
	h.ServeHTTP(httptest.NewRecorder(), req)
	if d := time.Since(start); d < 25*time.Millisecond {
		t.Fatalf("failure returned in %v (want delayed)", d)
	}
}

package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, candidate string
		want               bool
	}{
		{"0.3.2", "v0.4.0", true},
		{"0.3.2", "v0.4.0-beta.1", true}, // newer core beats prerelease
		{"0.4.0-beta.1", "0.4.0", true},  // release outranks its prereleases
		{"0.4.0", "0.4.0-beta.1", false},
		{"0.4.0-beta.1", "0.4.0-beta.2", true},
		{"0.4.0-beta.2", "0.4.0-beta.11", true}, // numeric compare, not lexical
		{"0.4.0-beta.1", "0.4.0-beta.1+meta", false},
		{"0.4.0", "0.4.0+build.5", false}, // build metadata ignored
		{"0.4.0", "v0.4.0", false},        // leading v tolerated
		{"1.2.3", "1.10.0", true},         // numeric compare, not lexical
		{"1.2.3", "1.2.3", false},
		// Anything unparsable never claims an update (dev builds never nag).
		{"dev", "v0.4.0", false},
		{"0.4.0", "", false},
		{"0.4.0", "not-a-version", false},
		{"", "0.4.0", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.current, c.candidate); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.candidate, got, c.want)
		}
	}
}

func TestCheckerFetchesCachesAndCompares(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 && r.URL.Path != "/repos/"+Repo+"/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://example.com/releases/v9.9.9"}`))
	}))
	defer srv.Close()

	c := &Checker{APIBase: srv.URL, TTL: time.Minute}
	res := c.Check(context.Background(), "0.3.2")
	if !res.Available || res.Latest != "v9.9.9" || res.Current != "0.3.2" || res.URL != "https://example.com/releases/v9.9.9" {
		t.Fatalf("res = %+v", res)
	}
	if res := c.Check(context.Background(), "9.9.9"); res.Available {
		t.Fatalf("same version must not flag: %+v", res)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("second check should be served from cache: hits=%d", n)
	}
}

func TestCheckerSwallowsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Checker{APIBase: srv.URL, TTL: time.Minute}
	if res := c.Check(context.Background(), "0.3.2"); res.Available {
		t.Fatalf("server error must collapse to available=false: %+v", res)
	}
	unreachable := &Checker{APIBase: "http://127.0.0.1:1", TTL: time.Minute}
	if res := unreachable.Check(context.Background(), "0.3.2"); res.Available {
		t.Fatalf("unreachable server must collapse to available=false: %+v", res)
	}
}

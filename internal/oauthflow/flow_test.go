package oauthflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/pleumcloud/pleumcloud/internal/secret"
)

type memSecret map[string][]byte

func (m memSecret) Set(ref string, data []byte) error { m[ref] = data; return nil }
func (m memSecret) Get(ref string) ([]byte, error) {
	d, ok := m[ref]
	if !ok {
		return nil, secret.ErrNotFound
	}
	return d, nil
}
func (m memSecret) Delete(ref string) error { delete(m, ref); return nil }

// TestStartCompleteRoundTrip drives the full loopback dance against a fake
// provider: Start → auth URL → (user "signs in") → callback with code →
// token exchange.
func TestStartCompleteRoundTrip(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("code") != "the-code" {
			t.Errorf("code = %q", r.FormValue("code"))
		}
		if pk := r.FormValue("code_verifier"); pk == "" {
			t.Error("PKCE verifier missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenSrv.Close()

	old := Specs["dropbox"]
	Specs["dropbox"] = Spec{
		AuthURL:  "https://auth.example/authorize",
		TokenURL: tokenSrv.URL,
		Scopes:   []string{"files.read"},
		UsePKCE:  true,
	}
	defer func() { Specs["dropbox"] = old }()

	m := NewManager(memSecret{})
	m.SetBYO("dropbox", ClientID{ID: "cid", Secret: "cs"})

	authURL, err := m.Start("dropbox", "http://127.0.0.1:7777")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	if u.Host != "auth.example" {
		t.Fatalf("auth host = %s", u.Host)
	}
	q := u.Query()
	if q.Get("client_id") != "cid" {
		t.Errorf("client_id = %q", q.Get("client_id"))
	}
	if !strings.HasPrefix(q.Get("redirect_uri"), "http://127.0.0.1:7777/api/connect/dropbox/callback") {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Error("PKCE challenge missing")
	}
	state := q.Get("state")
	if state == "" {
		t.Fatal("state missing")
	}

	cb, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:7777/api/connect/dropbox/callback?state="+state+"&code=the-code", nil)
	provider, tok, err := m.Complete(cb)
	if err != nil {
		t.Fatal(err)
	}
	if provider != "dropbox" || tok.AccessToken != "at" || tok.RefreshToken != "rt" {
		t.Fatalf("provider=%s tok=%+v", provider, tok)
	}

	// Replayed state must be rejected.
	cb2, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:7777/api/connect/dropbox/callback?state="+state+"&code=the-code", nil)
	if _, _, err := m.Complete(cb2); err == nil {
		t.Fatal("replay accepted")
	}
}

// TestMissingClientID verifies the setup guidance error.
func TestMissingClientID(t *testing.T) {
	unsetBuiltinEnv(t)
	m := NewManager(memSecret{})
	if _, err := m.Start("gdrive", "http://127.0.0.1:7777"); err != ErrNoClientID {
		t.Fatalf("err = %v", err)
	}
}

// TestPersistingSourceWritesRefresh ensures refreshed tokens survive to the
// secret store.
func TestPersistingSourceWritesRefresh(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-at","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()

	s := memSecret{}
	tok := &oauth2.Token{AccessToken: "old", RefreshToken: "r", Expiry: time.Now().Add(-time.Minute)}
	conf := &oauth2.Config{
		ClientID:     "cid",
		ClientSecret: "cs",
		Endpoint:     oauth2.Endpoint{AuthURL: "https://a", TokenURL: srv.URL},
	}
	src := &PersistingSource{secrets: s, ref: "acct:x", tok: tok, src: conf.TokenSource(context.Background(), tok)}
	got, err := src.Token()
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "new-at" || calls != 1 {
		t.Fatalf("token=%q calls=%d", got.AccessToken, calls)
	}
	buf, err := s.Get("acct:x")
	if err != nil || !strings.Contains(string(buf), "new-at") {
		t.Fatalf("persisted=%q err=%v", buf, err)
	}
}

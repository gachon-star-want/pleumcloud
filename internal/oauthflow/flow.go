// Package oauthflow implements the browser loopback OAuth2 dance for every
// OAuth provider: start (redirect to provider), callback (code exchange with
// PKCE where supported), and token persistence with auto-refresh.
package oauthflow

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/pleumcloud/pleumcloud/internal/secret"
)

// ErrNoClientID is returned when neither a built-in nor a BYO client is
// configured; the UI turns this into a setup prompt.
var ErrNoClientID = errors.New("no OAuth client configured for this provider — paste your own Client ID in settings")

// Spec describes one provider's OAuth endpoints.
type Spec struct {
	AuthURL  string   `json:"authUrl"`
	TokenURL string   `json:"tokenUrl"`
	Scopes   []string `json:"scopes"`
	UsePKCE  bool     `json:"usePkce"`
	// ExtraAuthParams are appended to the authorization request
	// (e.g. Dropbox's token_access_type=offline).
	ExtraAuthParams url.Values `json:"-"`
	// AccessTypeOffline adds Google's access_type=offline prompt=consent
	// pair so refresh tokens are issued reliably.
	AccessTypeOffline bool `json:"-"`
}

// Specs for every OAuth provider we speak natively.
var Specs = map[string]Spec{
	"gdrive": {
		AuthURL:           "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:          "https://oauth2.googleapis.com/token",
		Scopes:            []string{"https://www.googleapis.com/auth/drive"},
		AccessTypeOffline: true,
	},
	"onedrive": {
		AuthURL:  "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize",
		TokenURL: "https://login.microsoftonline.com/consumers/oauth2/v2.0/token",
		Scopes:   []string{"Files.ReadWrite.All", "offline_access", "User.Read"},
	},
	"dropbox": {
		AuthURL:  "https://www.dropbox.com/oauth2/authorize",
		TokenURL: "https://api.dropboxapi.com/oauth2/token",
		Scopes:   []string{"files.content.write", "files.content.read", "files.metadata.read", "account_info.read"},
		// Dropbox needs token_access_type=offline for refresh tokens and
		// its own account picker.
		ExtraAuthParams: url.Values{"token_access_type": {"offline"}},
	},
	"pcloud": {
		AuthURL:  "https://my.pcloud.com/oauth2/authorize",
		TokenURL: "https://api.pcloud.com/oauth2_token",
		Scopes:   nil, // pCloud grants full access per app registration
	},
}

// ClientID resolves the OAuth client for a provider: BYO credential first,
// then the built-in project app.
type ClientID struct {
	ID     string
	Secret string
}

type Manager struct {
	secrets secret.Store
	// builtin holds the project's official OAuth apps, injected at start
	// (empty until the apps are registered/approved).
	builtin map[string]ClientID
	// byo is loaded from the meta table by the API layer.
	byoMu sync.RWMutex
	byo   map[string]ClientID

	mu      sync.Mutex
	pending map[string]pendingFlow
}

type pendingFlow struct {
	provider     string
	verifier     string
	redirectBase string
	created      time.Time
}

func NewManager(secrets secret.Store) *Manager {
	return &Manager{
		secrets: secrets,
		builtin: map[string]ClientID{},
		byo:     map[string]ClientID{},
		pending: map[string]pendingFlow{},
	}
}

// SetBYO installs user-provided credentials for a provider.
func (m *Manager) SetBYO(provider string, c ClientID) {
	m.byoMu.Lock()
	m.byo[provider] = c
	m.byoMu.Unlock()
}

// HasBuiltin reports whether the project ships an official OAuth app for
// the provider.
func (m *Manager) HasBuiltin(provider string) bool {
	m.byoMu.RLock()
	defer m.byoMu.RUnlock()
	return m.builtin[provider].ID != ""
}

func (m *Manager) client(provider string) (ClientID, Spec, error) {
	spec, ok := Specs[provider]
	if !ok {
		return ClientID{}, Spec{}, fmt.Errorf("unknown oauth provider %q", provider)
	}
	m.byoMu.RLock()
	c, hasBYO := m.byo[provider]
	m.byoMu.RUnlock()
	if !hasBYO {
		c = m.builtin[provider]
	}
	if c.ID == "" {
		return ClientID{}, spec, ErrNoClientID
	}
	return c, spec, nil
}

func randToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// CallbackPath returns the redirect URI path registered with the provider.
func CallbackPath(provider string) string {
	return "/api/connect/" + provider + "/callback"
}

// Start builds the provider authorization URL and remembers the pending
// flow under a random state. redirectBase is where this server is reachable
// (e.g. http://127.0.0.1:7777).
func (m *Manager) Start(provider, redirectBase string) (string, error) {
	c, spec, err := m.client(provider)
	if err != nil {
		return "", err
	}

	state := randToken(24)
	pf := pendingFlow{provider: provider, redirectBase: redirectBase, created: time.Now()}
	q := url.Values{
		"client_id":     {c.ID},
		"redirect_uri":  {redirectBase + CallbackPath(provider)},
		"response_type": {"code"},
		"state":         {state},
	}
	for k, vs := range spec.ExtraAuthParams {
		q[k] = vs
	}
	if len(spec.Scopes) > 0 {
		q.Set("scope", strings.Join(spec.Scopes, " "))
	}
	if spec.UsePKCE {
		pf.verifier = randToken(48)
		sum := sha256Sum(pf.verifier)
		q.Set("code_challenge", sum)
		q.Set("code_challenge_method", "S256")
	}
	if spec.AccessTypeOffline {
		q.Set("access_type", "offline")
		q.Set("prompt", "consent")
	}

	m.mu.Lock()
	m.pruneLocked()
	m.pending[state] = pf
	m.mu.Unlock()

	authURL, err := url.Parse(spec.AuthURL)
	if err != nil {
		return "", err
	}
	authURL.RawQuery = q.Encode()
	return authURL.String(), nil
}

// pruneLocked drops flows older than 10 minutes; caller holds mu.
func (m *Manager) pruneLocked() {
	cutoff := time.Now().Add(-10 * time.Minute)
	for k, pf := range m.pending {
		if pf.created.Before(cutoff) {
			delete(m.pending, k)
		}
	}
}

// Complete validates the callback, exchanges the code, and returns the
// provider ID and the persisted-ready token.
func (m *Manager) Complete(r *http.Request) (string, *oauth2.Token, error) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		return "", nil, fmt.Errorf("provider returned error: %s (%s)", e, q.Get("error_description"))
	}
	state := q.Get("state")
	code := q.Get("code")
	if state == "" || code == "" {
		return "", nil, errors.New("missing state or code in callback")
	}

	m.mu.Lock()
	pf, ok := m.pending[state]
	if ok {
		delete(m.pending, state)
	}
	m.mu.Unlock()
	if !ok {
		return "", nil, errors.New("unknown or expired OAuth state — start the connection again")
	}

	c, spec, err := m.client(pf.provider)
	if err != nil {
		return "", nil, err
	}
	conf := &oauth2.Config{
		ClientID:     c.ID,
		ClientSecret: c.Secret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  spec.AuthURL,
			TokenURL: spec.TokenURL,
		},
		Scopes: spec.Scopes,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	var tok *oauth2.Token
	if spec.UsePKCE {
		tok, err = conf.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", pf.verifier))
	} else {
		tok, err = conf.Exchange(ctx, code)
	}
	if err != nil {
		return "", nil, fmt.Errorf("token exchange: %w", err)
	}
	return pf.provider, tok, nil
}

// PersistingSource wraps an oauth2 token so every refresh is written back
// to the secret store, keeping restarts and multi-account tokens honest.
type PersistingSource struct {
	src     oauth2.TokenSource
	secrets secret.Store
	ref     string

	mu   sync.Mutex
	tok  *oauth2.Token
	stop bool
}

// NewClient returns an http.Client whose token refreshes are persisted.
func NewClient(ctx context.Context, secrets secret.Store, ref string, conf *oauth2.Config, tok *oauth2.Token) *http.Client {
	ps := &PersistingSource{secrets: secrets, ref: ref, tok: tok}
	ps.src = conf.TokenSource(ctx, tok)
	return oauth2.NewClient(ctx, ps)
}

func (p *PersistingSource) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	tok, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	if tok.AccessToken != p.tok.AccessToken {
		buf, _ := json.Marshal(tok)
		if err := p.secrets.Set(p.ref, buf); err != nil {
			// Refresh worked; persistence failed. Surface but don't kill the call.
			return tok, fmt.Errorf("token refreshed but not persisted: %w", err)
		}
		p.tok = tok
	}
	return tok, nil
}

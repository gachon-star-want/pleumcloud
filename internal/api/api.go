// Package api exposes the REST surface consumed by the embedded SPA
// (and, post-MVP, the mobile PWA against server mode).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/gachon-star-want/pleumcloud/internal/auth"
	"github.com/gachon-star-want/pleumcloud/internal/index"
	"github.com/gachon-star-want/pleumcloud/internal/oauthflow"
	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
	"github.com/gachon-star-want/pleumcloud/internal/store"
)

// API bundles handler dependencies.
type API struct {
	store       *store.Store
	secrets     secret.Store
	oauth       *oauthflow.Manager
	indexer     *index.Indexer
	tokens      *auth.TokenKey
	multiuser   bool
	localUserID string
	thumbDir    string
	version     string
}

// New wires the API handlers.
func New(st *store.Store, secrets secret.Store, oauth *oauthflow.Manager, idx *index.Indexer, tokens *auth.TokenKey, multiuser bool, version string) *API {
	return &API{store: st, secrets: secrets, oauth: oauth, indexer: idx, tokens: tokens, multiuser: multiuser, version: version}
}

// ---- authentication ----

const cookieName = "pcu"

type ctxKey int

const userKey ctxKey = 1

// InitLocalUser ensures the implicit local-mode owner exists.
func (a *API) InitLocalUser() error {
	id, err := a.store.EnsureUser("local", "")
	if err != nil {
		return err
	}
	if err := a.store.AdoptLegacyLocalRows(id); err != nil {
		return err
	}
	a.localUserID = id
	return nil
}

// ctxUser resolves the acting user. Local mode is always the implicit
// local user; multiuser mode requires a valid cookie or Bearer token.
func (a *API) ctxUser(r *http.Request) (string, error) {
	if !a.multiuser {
		return a.localUserID, nil
	}
	if tok := bearerToken(r); tok != "" {
		if sub, err := a.tokens.Verify(tok); err == nil {
			return sub, nil
		}
	}
	if c, err := r.Cookie(cookieName); err == nil {
		if sub, err := a.tokens.Verify(c.Value); err == nil {
			return sub, nil
		}
	}
	return "", fmt.Errorf("sign in required")
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	return strings.TrimPrefix(h, "Bearer ")
}

// requireUser wraps handlers that need an acting user.
func (a *API) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := a.ctxUser(r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	}
}

func userID(r *http.Request) string {
	u, _ := r.Context().Value(userKey).(string)
	return u
}

func (a *API) setSessionCookie(w http.ResponseWriter, tok string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600,
	})
}

// SetDataDir points the thumbnail cache at the data directory.
func (a *API) SetDataDir(dir string) { a.thumbDir = filepath.Join(dir, "thumbs") }

// LoadBYOCredentials reads stored BYO OAuth clients into the flow manager
// (called once at startup).
func (a *API) LoadBYOCredentials() error {
	for id := range oauthflow.Specs {
		cid, err := a.store.LoadMeta("oauth:byo:" + id + ":id")
		if err != nil {
			return err
		}
		if cid == "" {
			continue
		}
		c := oauthflow.ClientID{ID: cid}
		if buf, err := a.secrets.Get("byo-secret:" + id); err == nil {
			c.Secret = string(buf)
		}
		a.oauth.SetBYO(id, c)
	}
	return nil
}

// Routes mounts all API endpoints under /api.
func (a *API) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/health", a.health)
	r.Get("/providers", a.providers)

	// Auth surface (public).
	r.Get("/auth/mode", a.authMode)
	r.Post("/auth/register", a.register)
	r.Post("/auth/login", a.login)
	r.Post("/auth/logout", a.logout)
	r.Get("/auth/me", a.requireUser(a.me))

	// Everything below is user-scoped.
	r.Get("/accounts", a.requireUser(a.accounts))
	r.Post("/accounts", a.requireUser(a.createAccount))
	r.Delete("/accounts/{id}", a.requireUser(a.deleteAccount))

	r.Get("/credentials", a.credentials)
	r.Put("/credentials/{provider}", a.putCredentials)

	r.Get("/connect/{provider}/start", a.requireUser(a.connectStart))
	r.Get("/connect/{provider}/callback", a.connectCallback)

	// Unified file operations (browsing, transfer, placement, rules).
	a.registerFileRoutes(r)
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	err := a.store.Ping()
	status := "ok"
	if err != nil {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  status,
		"version": a.version,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (a *API) providers(w http.ResponseWriter, r *http.Request) {
	type pv struct {
		provider.Metadata
		Supported bool `json:"supported"`
	}
	out := make([]pv, 0, len(provider.Catalog))
	for _, m := range provider.Catalog {
		out = append(out, pv{Metadata: m, Supported: provider.Supported(m.ID)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

// ---- accounts ----

type accountReq struct {
	ProviderID string `json:"providerId"`
	Method     string `json:"method"` // pat | webdav | mediafire
	Token      string `json:"token"`  // pat
	URL        string `json:"url"`    // webdav
	Username   string `json:"username"`
	Password   string `json:"password"`
	Label      string `json:"label"`
	AppID      string `json:"appId"`  // mediafire
	APIKey     string `json:"apiKey"` // mediafire
}

func (a *API) accounts(w http.ResponseWriter, r *http.Request) {
	accts, err := a.store.ListAccountsForUser(userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if accts == nil {
		accts = []store.Account{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accts})
}

func (a *API) createAccount(w http.ResponseWriter, r *http.Request) {
	var req accountReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("invalid JSON body"))
		return
	}

	id, err := store.NewAccountID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	ref := "acct:" + id

	switch req.Method {
	case "pat":
		if req.Token == "" {
			writeErr(w, http.StatusBadRequest, errors.New("token is required"))
			return
		}
		if err := secret.PutJSON(a.secrets, ref, map[string]string{"pat": req.Token, "email": req.Username}); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	case "mediafire":
		if req.Username == "" || req.Password == "" || req.AppID == "" || req.APIKey == "" {
			writeErr(w, http.StatusBadRequest, errors.New("email, password, app id and api key are required"))
			return
		}
		if err := secret.PutJSON(a.secrets, ref, map[string]string{
			"email": req.Username, "password": req.Password, "appId": req.AppID, "apiKey": req.APIKey,
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	case "webdav":
		if req.URL == "" {
			writeErr(w, http.StatusBadRequest, errors.New("url is required"))
			return
		}
		if err := secret.PutJSON(a.secrets, ref, map[string]string{
			"url": req.URL, "username": req.Username, "password": req.Password,
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unsupported method %q (use the OAuth flow for %s)", req.Method, req.ProviderID))
		return
	}

	// Validate by exercising the connector; on failure, drop the secret.
	conn, ok := provider.Build(req.ProviderID, provider.Deps{Secrets: a.secrets})
	if !ok {
		a.secrets.Delete(ref)
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no connector for provider %q", req.ProviderID))
		return
	}
	acct := provider.AccountRef{ID: id, ProviderID: req.ProviderID, SecretRef: ref}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	_, qerr := conn.Quota(ctx, acct)
	switch {
	case qerr == nil:
		// quota worked; token is good
	case errors.Is(qerr, provider.ErrUnsupported):
		// No quota API (WebDAV): a root listing validates credentials.
		if _, _, lerr := conn.List(ctx, acct, "", ""); lerr != nil {
			a.secrets.Delete(ref)
			writeErr(w, http.StatusUnauthorized, fmt.Errorf("validation failed: %v", lerr))
			return
		}
	default:
		a.secrets.Delete(ref)
		writeErr(w, http.StatusUnauthorized, fmt.Errorf("validation failed: %v", qerr))
		return
	}

	label := req.Label
	if label == "" {
		label = defaultLabel(req.ProviderID)
	}
	if err := a.store.AddAccountWithIDForUser(id, userID(r), req.ProviderID, label, req.Method, ref); err != nil {
		a.secrets.Delete(ref)
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Files should appear without waiting for the background sync round.
	go a.syncAccount(id)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "label": label})
}

func defaultLabel(providerID string) string {
	for _, m := range provider.Catalog {
		if m.ID == providerID {
			return m.Name
		}
	}
	return providerID
}

func (a *API) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := a.store.GetAccountForUser(id, userID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, errors.New("account not found"))
		return
	}
	if err := a.store.DeleteAccountForUser(id, userID(r)); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	_ = a.secrets.Delete(row.SecretRef)
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

// ---- auth endpoints ----

func (a *API) authMode(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"multiuser": a.multiuser})
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	if !a.multiuser {
		writeErr(w, http.StatusForbidden, errors.New("registration is disabled in local mode"))
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := jsonDecode(r, &req); err != nil || req.Email == "" || len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, errors.New("email and a password of 8+ characters are required"))
		return
	}
	if _, err := a.store.UserByEmail(strings.ToLower(req.Email)); err == nil {
		writeErr(w, http.StatusConflict, errors.New("email already registered"))
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	id, err := a.store.EnsureUser(strings.ToLower(req.Email), string(hash))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	a.issueSession(w, id, req.Email)
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if !a.multiuser {
		writeErr(w, http.StatusForbidden, errors.New("login is disabled in local mode"))
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := jsonDecode(r, &req); err != nil || req.Email == "" {
		writeErr(w, http.StatusBadRequest, errors.New("email and password are required"))
		return
	}
	u, err := a.store.UserByEmail(strings.ToLower(req.Email))
	if err != nil {
		time.Sleep(300 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, errors.New("invalid credentials"))
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		time.Sleep(300 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, errors.New("invalid credentials"))
		return
	}
	a.issueSession(w, u.ID, u.Email)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"multiuser": a.multiuser})
}

func (a *API) issueSession(w http.ResponseWriter, id, email string) {
	tok, err := a.tokens.Issue(id, 30*24*time.Hour)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	a.setSessionCookie(w, tok)
	writeJSON(w, http.StatusOK, map[string]any{"email": email})
}

// ---- BYO OAuth credentials ----

func (a *API) credentials(w http.ResponseWriter, r *http.Request) {
	type cred struct {
		Provider   string `json:"provider"`
		Configured bool   `json:"configured"` // BYO or built-in app present
		HasBYO     bool   `json:"hasByo"`
		ClientID   string `json:"clientId,omitempty"`
	}
	out := []cred{}
	for id := range oauthflow.Specs {
		c := cred{Provider: id}
		if cid, _ := a.store.LoadMeta("oauth:byo:" + id + ":id"); cid != "" {
			c.HasBYO = true
			c.ClientID = cid
			c.Configured = true
		} else if a.oauth.HasBuiltin(id) {
			c.Configured = true
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": out})
}

func (a *API) putCredentials(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "provider")
	if _, ok := oauthflow.Specs[id]; !ok {
		writeErr(w, http.StatusNotFound, errors.New("unknown OAuth provider"))
		return
	}
	var req struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ClientID == "" {
		writeErr(w, http.StatusBadRequest, errors.New("clientId is required"))
		return
	}
	if err := a.store.SaveMeta("oauth:byo:"+id+":id", req.ClientID); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if req.ClientSecret != "" {
		if err := a.secrets.Set("byo-secret:"+id, []byte(req.ClientSecret)); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	}
	a.oauth.SetBYO(id, oauthflow.ClientID{ID: req.ClientID, Secret: req.ClientSecret})
	writeJSON(w, http.StatusOK, map[string]string{"provider": id, "status": "saved"})
}

// ---- OAuth loopback ----

// connectErrorPage renders a small readable error page for browser-facing
// OAuth failures (the SPA's i18n never reaches these round-trip pages, so
// guidance ships bilingually). Provider-sourced text is escaped.
func connectErrorPage(w http.ResponseWriter, status int, title string, provider string, err error) {
	hint := ""
	msg := html.EscapeString(err.Error())
	switch {
	case strings.Contains(msg, "invalid_client"):
		hint = fmt.Sprintf("The OAuth client ID/secret was rejected by %s. If you pasted your own app key, re-copy it exactly from the provider console — docs/oauth-setup.md has the guide.<br>클라이언트 ID/시크릿이 %s에서 거부되었습니다. 직접 붙여넣은 앱 키라면 콘솔에서 정확히 다시 복사해 주세요 (안내: docs/oauth-setup.md).", provider, provider)
	case strings.Contains(msg, "access_denied"):
		hint = "The sign-in was cancelled at the provider.<br>제공사 화면에서 로그인이 취소되었습니다."
	case strings.Contains(msg, "unknown or expired OAuth state"):
		hint = "The sign-in window expired (10 minutes) — start the connection again.<br>로그인 세션이 만료되었습니다(10분). 연결을 다시 시작해 주세요."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>%s</title>
<body style="font-family:system-ui,sans-serif;padding:3rem;max-width:40rem;margin:auto">
<h3>%s</h3><p>%s</p>%s
<p><a href="/connect">Try again / 다시 시도</a> · <a href="/">Back to PleumCloud</a></p></body>`,
		html.EscapeString(title), html.EscapeString(title), msg, hint)
}

func (a *API) connectStart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "provider")
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	redirectBase := scheme + "://" + r.Host
	authURL, err := a.oauth.Start(id, redirectBase)
	if err != nil {
		// Browser-facing: a small readable error beats JSON here.
		connectErrorPage(w, http.StatusBadRequest, "PleumCloud — can't start "+id+" connection", id, err)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (a *API) connectCallback(w http.ResponseWriter, r *http.Request) {
	id, tok, err := a.oauth.Complete(r)
	if err != nil {
		connectErrorPage(w, http.StatusBadGateway, "PleumCloud — connection failed", id, err)
		return
	}

	acctID, err := store.NewAccountID()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "<h3>PleumCloud — internal error</h3><p>%s</p>", err)
		return
	}
	ref := "acct:" + acctID
	if err := secret.PutJSON(a.secrets, ref, map[string]any{"token": tok}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "<h3>PleumCloud — could not save credentials</h3><p>%s</p>", err)
		return
	}

	// Best-effort label from the provider (e.g. the Google account email).
	label := defaultLabel(id)
	acct := provider.AccountRef{ID: acctID, ProviderID: id, SecretRef: ref}
	if conn, ok := provider.Build(id, provider.Deps{Secrets: a.secrets}); ok {
		if lc, ok := conn.(interface {
			AccountLabel(ctx context.Context, acct provider.AccountRef) (string, error)
		}); ok {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			if l, err := lc.AccountLabel(ctx, acct); err == nil && l != "" {
				label = l
			}
			cancel()
		}
	}

	cbUser := a.localUserID
	if a.multiuser {
		if c, err := r.Cookie(cookieName); err == nil {
			if sub, err := a.tokens.Verify(c.Value); err == nil {
				cbUser = sub
			}
		}
	}
	if err := a.store.AddAccountWithIDForUser(acctID, cbUser, id, label, "oauth2", ref); err != nil {
		a.secrets.Delete(ref)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "<h3>PleumCloud — could not save the account</h3><p>%s</p>", err)
		return
	}
	// Files should appear without waiting for the background sync round.
	go a.syncAccount(acctID)
	http.Redirect(w, r, "/?connected="+id, http.StatusFound)
}

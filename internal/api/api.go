// Package api exposes the REST surface consumed by the embedded SPA
// (and, post-MVP, the mobile PWA against server mode).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pleumcloud/pleumcloud/internal/oauthflow"
	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/secret"
	"github.com/pleumcloud/pleumcloud/internal/store"
)

// API bundles handler dependencies.
type API struct {
	store    *store.Store
	secrets  secret.Store
	oauth    *oauthflow.Manager
	version  string
}

// New wires the API handlers.
func New(st *store.Store, secrets secret.Store, oauth *oauthflow.Manager, version string) *API {
	return &API{store: st, secrets: secrets, oauth: oauth, version: version}
}

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

	r.Get("/accounts", a.accounts)
	r.Post("/accounts", a.createAccount)
	r.Delete("/accounts/{id}", a.deleteAccount)

	r.Get("/credentials", a.credentials)
	r.Put("/credentials/{provider}", a.putCredentials)

	r.Get("/connect/{provider}/start", a.connectStart)
	r.Get("/connect/{provider}/callback", a.connectCallback)
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
	Method     string `json:"method"` // pat | webdav
	Token      string `json:"token"`  // pat
	URL        string `json:"url"`    // webdav
	Username   string `json:"username"`
	Password   string `json:"password"`
	Label      string `json:"label"`
}

func (a *API) accounts(w http.ResponseWriter, r *http.Request) {
	accts, err := a.store.ListAccounts()
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
		if err := secret.PutJSON(a.secrets, ref, map[string]string{"pat": req.Token}); err != nil {
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
	if err := a.store.AddAccountWithID(id, req.ProviderID, label, req.Method, ref); err != nil {
		a.secrets.Delete(ref)
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
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
	row, err := a.store.GetAccount(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, errors.New("account not found"))
		return
	}
	if err := a.store.DeleteAccount(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	_ = a.secrets.Delete(row.SecretRef)
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
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
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "<h3>PleumCloud — can't start %s connection</h3><p>%s</p><p><a href='/'>Back</a></p>", id, err)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (a *API) connectCallback(w http.ResponseWriter, r *http.Request) {
	id, tok, err := a.oauth.Complete(r)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "<h3>PleumCloud — connection failed</h3><p>%s</p><p><a href='/connect'>Try again</a> · <a href='/'>Back</a></p>", err)
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

	if err := a.store.AddAccountWithID(acctID, id, label, "oauth2", ref); err != nil {
		a.secrets.Delete(ref)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "<h3>PleumCloud — could not save the account</h3><p>%s</p>", err)
		return
	}
	http.Redirect(w, r, "/?connected="+id, http.StatusFound)
}

// Package api exposes the REST surface consumed by the embedded SPA
// (and, post-MVP, the mobile PWA against server mode).
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/store"
)

// API bundles handler dependencies.
type API struct {
	store   *store.Store
	version string
}

func New(st *store.Store, version string) *API { return &API{store: st, version: version} }

// Routes mounts all API endpoints under /api.
func (a *API) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/health", a.health)
	r.Get("/providers", a.providers)
	r.Get("/accounts", a.accounts)
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
	writeJSON(w, http.StatusOK, map[string]any{"providers": provider.Catalog})
}

func (a *API) accounts(w http.ResponseWriter, r *http.Request) {
	accts, err := a.store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if accts == nil {
		accts = []store.Account{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accts})
}

// Package server wires the HTTP server: REST API under /api and the
// embedded SPA for everything else, with history fallback for client-side
// routing.
package server

import (
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/pleumcloud/pleumcloud/internal/api"
	"github.com/pleumcloud/pleumcloud/internal/config"
	"github.com/pleumcloud/pleumcloud/internal/store"
	web "github.com/pleumcloud/pleumcloud/web"
)

// Server owns the HTTP stack.
type Server struct {
	cfg     *config.Config
	handler http.Handler
}

// New builds the server. In dev the SPA assets come from web/dist (rebuilt
// by `make build` or `cd web && npm run build`); releases embed the built
// assets into the binary.
func New(cfg *config.Config, st *store.Store, version string) *Server {
	a := api.New(st, version)

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP)
	r.Use(middleware.Logger, middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))

	r.Mount("/api", a.Routes())

	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		panic("embedded web/dist missing: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(dist))
	r.Get("/*", spaFallback(dist, fileServer))

	return &Server{cfg: cfg, handler: r}
}

// spaFallback serves static files, falling back to index.html for paths the
// SPA router owns (anything without a file extension).
func spaFallback(dist fs.FS, fileServer http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil && !strings.Contains(p, ".") {
			// SPA route — serve the shell.
			index, err2 := fs.ReadFile(dist, "index.html")
			if err2 != nil {
				http.Error(w, "frontend not built", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(index)
			return
		}
		fileServer.ServeHTTP(w, r)
	}
}

// ListenAndServe starts the HTTP server on addr.
func (s *Server) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return http.Serve(ln, s.handler)
}

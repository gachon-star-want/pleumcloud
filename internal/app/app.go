// Package app assembles the PleumCloud core — store, secrets, index, API,
// background workers and HTTP server — behind a single Start/Close pair,
// so the CLI today and embedders next (desktop shell) run the identical
// app in-process.
package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gachon-star-want/pleumcloud/internal/api"
	"github.com/gachon-star-want/pleumcloud/internal/auth"
	"github.com/gachon-star-want/pleumcloud/internal/config"
	"github.com/gachon-star-want/pleumcloud/internal/index"
	"github.com/gachon-star-want/pleumcloud/internal/oauthflow"
	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
	"github.com/gachon-star-want/pleumcloud/internal/server"
	"github.com/gachon-star-want/pleumcloud/internal/store"

	// Connectors self-register into the provider registry; importing them
	// here means every embedder gets the full catalog.
	_ "github.com/gachon-star-want/pleumcloud/internal/provider/bridge"
	_ "github.com/gachon-star-want/pleumcloud/internal/provider/drime"
	_ "github.com/gachon-star-want/pleumcloud/internal/provider/dropbox"
	_ "github.com/gachon-star-want/pleumcloud/internal/provider/gdrive"
	_ "github.com/gachon-star-want/pleumcloud/internal/provider/koofr"
	_ "github.com/gachon-star-want/pleumcloud/internal/provider/mediafire"
	_ "github.com/gachon-star-want/pleumcloud/internal/provider/mybox"
	_ "github.com/gachon-star-want/pleumcloud/internal/provider/onedrive"
	_ "github.com/gachon-star-want/pleumcloud/internal/provider/pcloud"
	_ "github.com/gachon-star-want/pleumcloud/internal/provider/webdav"
)

// Options are the knobs an embedder sets; zero values fall back to
// config.Load() and "dev".
type Options struct {
	// Version is reported by the API and the banner (build-time ldflags).
	Version string
	// Config overrides environment-derived configuration. Used by tests
	// and embedders that own their own settings surface.
	Config *config.Config
	// Middleware optionally wraps the HTTP handler. The desktop shell uses
	// it to serve desktop-only endpoints (e.g. open-in-system-browser)
	// next to the core routes. Nil keeps the handler unwrapped.
	Middleware func(http.Handler) http.Handler
}

// App is a running PleumCloud core: HTTP server plus background workers.
type App struct {
	Cfg     *config.Config
	Store   *store.Store
	Secrets secret.Store
	Index   *index.Indexer
	API     *api.API

	// Addr is the bound listen address; URL is where the UI lives
	// (wildcard binds normalize to loopback).
	Addr string
	URL  string

	httpSrv  *http.Server
	cancel   context.CancelFunc
	workers  sync.WaitGroup
	closeOne sync.Once
	closeErr error
}

// Start assembles the core, binds the listener, launches the background
// workers and serves HTTP in the background. The caller owns the
// lifecycle via Close.
func Start(opts Options) (*App, error) {
	cfg := opts.Config
	if cfg == nil {
		var err error
		if cfg, err = config.Load(); err != nil {
			return nil, fmt.Errorf("config: %w", err)
		}
	}
	version := opts.Version
	if version == "" {
		version = "dev"
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("data dir: %w", err)
	}

	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	var secrets secret.Store = secret.New(cfg.DataDir)
	if cfg.MultiUser {
		// Server deployments deserve encryption at rest even on the file
		// fallback; keychain users already get OS protection.
		es, err := secret.NewEncryptedFileStore(cfg.DataDir)
		if err != nil {
			st.Close()
			return nil, fmt.Errorf("secret store: %w", err)
		}
		secrets = es
	}
	oauth := oauthflow.NewManager(secrets)
	idx := index.New(st)
	tokens, err := auth.LoadOrCreateTokenKey(filepath.Join(cfg.DataDir, "auth.key"))
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("token key: %w", err)
	}

	restAPI := api.New(st, secrets, oauth, idx, tokens, cfg.MultiUser, version)
	restAPI.SetDataDir(cfg.DataDir)
	if err := restAPI.InitLocalUser(); err != nil {
		st.Close()
		return nil, fmt.Errorf("local user: %w", err)
	}
	if err := restAPI.LoadBYOCredentials(); err != nil {
		st.Close()
		return nil, fmt.Errorf("load credentials: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	a := &App{
		Cfg:     cfg,
		Store:   st,
		Secrets: secrets,
		Index:   idx,
		API:     restAPI,
		cancel:  cancel,
	}

	// Background workers: keep the unified index fresh, drain transfers.
	a.workers.Add(2)
	go func() { defer a.workers.Done(); a.syncLoop(ctx) }()
	go func() { defer a.workers.Done(); a.transferLoop(ctx) }()

	ln, err := net.Listen("tcp", cfg.BindAddr())
	if err != nil {
		cancel()
		a.workers.Wait()
		st.Close()
		return nil, fmt.Errorf("listen: %w", err)
	}

	handler := server.New(cfg, restAPI).Handler()
	if opts.Middleware != nil {
		handler = opts.Middleware(handler)
	}
	a.httpSrv = &http.Server{Handler: handler}
	go func() { _ = a.httpSrv.Serve(ln) }()

	a.Addr = ln.Addr().String()
	a.URL = "http://" + net.JoinHostPort(hostForURL(cfg.Bind), portOf(ln))
	return a, nil
}

// Close shuts the HTTP server and background workers down and closes the
// store. Safe to call more than once.
func (a *App) Close() error {
	a.closeOne.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if a.httpSrv != nil {
			a.closeErr = a.httpSrv.Shutdown(ctx)
		}
		a.cancel()
		waited := make(chan struct{})
		go func() { a.workers.Wait(); close(waited) }()
		select {
		case <-waited:
		case <-time.After(2 * time.Second):
			log.Print("app: workers did not stop within 2s; continuing shutdown")
		}
		if err := a.Store.Close(); err != nil && a.closeErr == nil {
			a.closeErr = err
		}
	})
	return a.closeErr
}

// hostForURL normalizes wildcard binds to loopback for the user-facing URL.
func hostForURL(bind string) string {
	if bind == "0.0.0.0" || bind == "::" || bind == "[::]" {
		return "127.0.0.1"
	}
	return bind
}

func portOf(ln net.Listener) string {
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok {
		return strconv.Itoa(tcp.Port)
	}
	return ln.Addr().String()
}

// syncLoop refreshes every account's index: shortly after startup, then
// every 5 minutes. One flaky account must never take the server down, so
// each round runs under recover and per-account errors are logged.
func (a *App) syncLoop(ctx context.Context) {
	deps := provider.Deps{Secrets: a.Secrets}
	run := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		errs := a.Index.SyncAll(rctx, deps, provider.Build)
		for acct, err := range errs {
			log.Printf("sync: account %s: %v", acct, err)
		}
		return errs["*"]
	}
	delay := time.NewTimer(3 * time.Second)
	defer delay.Stop()
	select {
	case <-ctx.Done():
		return
	case <-delay.C:
	}
	if err := run(); err != nil {
		log.Printf("sync round failed: %v", err)
	}
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := run(); err != nil {
				log.Printf("sync round failed: %v", err)
			}
		}
	}
}

// transferLoop drains cross-cloud jobs sequentially, streaming source →
// server → destination so phones can drop off mid-transfer.
func (a *App) transferLoop(ctx context.Context) {
	for {
		job, err := a.Store.ClaimNextQueuedJob()
		if err != nil || job == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		a.runTransfer(ctx, job)
	}
}

func (a *App) runTransfer(ctx context.Context, job *store.JobRow) {
	fail := func(msg string) { _ = a.Store.FinishJob(job.ID, "failed", msg) }

	srcRow, err := a.Store.GetAccount(job.SrcAccount)
	if err != nil {
		fail("source account missing")
		return
	}
	dstRow, err := a.Store.GetAccount(job.DstAccount)
	if err != nil {
		fail("destination account missing")
		return
	}
	srcConn, ok := provider.Build(srcRow.ProviderID, provider.Deps{Secrets: a.Secrets})
	if !ok {
		fail("no source connector")
		return
	}
	dstConn, ok := provider.Build(dstRow.ProviderID, provider.Deps{Secrets: a.Secrets})
	if !ok {
		fail("no destination connector")
		return
	}
	srcRef := provider.AccountRef{ID: srcRow.ID, ProviderID: srcRow.ProviderID, SecretRef: srcRow.SecretRef}
	dstRef := provider.AccountRef{ID: dstRow.ID, ProviderID: dstRow.ProviderID, SecretRef: dstRow.SecretRef}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()

	rc, err := srcConn.Open(ctx, srcRef, job.SrcRemote, nil)
	if err != nil {
		fail(err.Error())
		return
	}
	defer rc.Close()

	progress := func(done, total int64) {
		if total <= 0 {
			total = job.TotalBytes
		}
		_ = a.Store.UpdateJobProgress(job.ID, done, total)
	}
	if _, err := dstConn.Upload(ctx, dstRef, "", job.FileName, rc, job.TotalBytes, progress); err != nil {
		fail(err.Error())
		return
	}
	_ = a.Store.FinishJob(job.ID, "done", "")
}

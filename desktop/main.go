//go:build darwin || windows

// PleumCloud desktop shell (D13): a Wails v2 window around the identical
// loopback core the web binary runs. The webview loads http://127.0.0.1:<port>
// so OAuth redirect URIs, cookies and streaming behave exactly like the web
// app. Pages served from the loopback origin cannot call Wails bindings
// (those are origin-bound), so the shell also mounts /__desktop/external —
// the SPA posts a URL there when it must open the system browser instead of
// navigating the webview (OAuth consent: Google blocks embedded webviews;
// file downloads: WKWebView cannot save).
package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gachon-star-want/pleumcloud/internal/app"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend
var assets embed.FS

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// Shell is the binding surface for the redirector page. When the core
// fails to start (e.g. port 7777 already in use) the redirector shows the
// error instead of navigating.
type Shell struct {
	url  string
	err  string
	core *app.App

	mu  sync.Mutex
	ctx context.Context
}

func (s *Shell) URL() string { return s.url }
func (s *Shell) Err() string { return s.err }

func main() {
	sh := &Shell{}
	core, err := app.Start(app.Options{Version: version, Middleware: sh.middleware})
	if err != nil {
		sh.err = startupHint(err)
	} else {
		defer core.Close()
		sh.url = core.URL
		sh.core = core
	}

	if err := wails.Run(&options.App{
		Title:             "PleumCloud",
		Width:             1280,
		Height:            800,
		MinWidth:          960,
		MinHeight:         600,
		AssetServer:       &assetserver.Options{Assets: assets},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "pleumcloud-desktop-3f8a1d2c-9b4e-4f7a-8c55-2d1e6b9a0f14",
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				if ctx := sh.browserCtx(); ctx != nil {
					wruntime.WindowUnminimise(ctx)
					wruntime.WindowShow(ctx)
				}
			},
		},
		OnStartup: func(ctx context.Context) {
			sh.mu.Lock()
			sh.ctx = ctx
			sh.mu.Unlock()
		},
		OnBeforeClose: sh.confirmQuit,
		Bind: []interface{}{sh},
		Mac: &mac.Options{
			About: &mac.AboutInfo{
				Title:   "PleumCloud",
				Message: fmt.Sprintf("all your free cloud storage, one drive (%s)\nAGPL-3.0", version),
			},
		},
	}); err != nil {
		log.Fatalf("shell: %v", err)
	}
}

func (s *Shell) browserCtx() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ctx
}

// confirmQuit guards against quitting mid-transfer: transfers stream
// through this process, so closing the window kills them. Returns true
// (block close) unless the user explicitly quits. Fails open when the
// core never started or the count errors — quitting stays the user's call.
func (s *Shell) confirmQuit(ctx context.Context) bool {
	if s.core == nil {
		return false
	}
	n, err := s.core.Store.CountActiveJobs()
	if err != nil || n == 0 {
		return false
	}
	choice, _ := wruntime.MessageDialog(ctx, wruntime.MessageDialogOptions{
		Type:          wruntime.QuestionDialog,
		Title:         "Transfers in progress",
		Message:       fmt.Sprintf("%d transfer(s) are still running — quitting cancels them. Quit anyway?", n),
		Buttons:       []string{"Quit", "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	return choice != "Quit"
}

// startupHint turns the raw bind error into an actionable message: OAuth
// provider consoles register redirect URIs with the port baked in, so there
// is no silent port fallback.
func startupHint(err error) string {
	if strings.Contains(err.Error(), "listen") {
		return "port 7777 is already in use — stop the other PleumCloud " +
			"instance (or set PLEUMCLOUD_PORT and re-register OAuth redirect URIs). Detail: " + err.Error()
	}
	return err.Error()
}

// middleware wraps the core handler with the desktop-only endpoint.
func (s *Shell) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__desktop/external" && r.Method == http.MethodPost {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<10))
			target := strings.TrimSpace(string(body))
			ctx := s.browserCtx()
			if ctx == nil || !isWebURL(target) {
				http.Error(w, "cannot open url", http.StatusServiceUnavailable)
				return
			}
			wruntime.BrowserOpenURL(ctx, target)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isWebURL only lets http(s) targets through — never file:// or app schemes.
func isWebURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

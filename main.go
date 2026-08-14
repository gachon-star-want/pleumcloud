// PleumCloud — all your free cloud storage, one drive.
// Copyright (C) 2026 PleumCloud contributors. AGPL-3.0.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/pleumcloud/pleumcloud/internal/api"
	"github.com/pleumcloud/pleumcloud/internal/browser"
	"github.com/pleumcloud/pleumcloud/internal/config"
	"github.com/pleumcloud/pleumcloud/internal/oauthflow"
	"github.com/pleumcloud/pleumcloud/internal/secret"
	"github.com/pleumcloud/pleumcloud/internal/server"
	"github.com/pleumcloud/pleumcloud/internal/store"

	// Connectors self-register into the provider registry.
	_ "github.com/pleumcloud/pleumcloud/internal/provider/dropbox"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/gdrive"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/mybox"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/onedrive"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/webdav"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	noBrowser := flag.Bool("no-browser", false, "don't open the browser on startup")
	flag.Parse()
	if *showVersion {
		fmt.Println("pleumcloud", version)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if *noBrowser {
		cfg.NoBrowser = true
	}

	st, err := store.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer st.Close()

	secrets := secret.New(cfg.DataDir)
	oauth := oauthflow.NewManager(secrets)

	a := api.New(st, secrets, oauth, version)
	if err := a.LoadBYOCredentials(); err != nil {
		log.Fatalf("load credentials: %v", err)
	}

	srv := server.New(cfg, a)
	addr := cfg.BindAddr()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}

	url := cfg.LocalURL()
	fmt.Printf(`
  ┌─────────────────────────────────────────────┐
  │                                             │
  │   PleumCloud — one drive for all your      │
  │   free cloud storage                        │
  │                                             │
  │   version %-34s │
  │                                             │
  └─────────────────────────────────────────────┘

  %s
  Data directory: %s
  Press Ctrl+C to stop.
`, version, url, cfg.DataDir)

	// Open the default browser only after the listener is bound, so the
	// page is guaranteed to load.
	if !cfg.NoBrowser {
		if err := browser.Open(url); err != nil {
			fmt.Printf("  (couldn't open a browser: %v — open %s manually)\n", err, url)
		}
	}

	if err := srv.Serve(ln); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

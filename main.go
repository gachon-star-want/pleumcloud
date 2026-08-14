// PleumCloud — all your free cloud storage, one drive.
// Copyright (C) 2026 PleumCloud contributors. AGPL-3.0.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pleumcloud/pleumcloud/internal/config"
	"github.com/pleumcloud/pleumcloud/internal/server"
	"github.com/pleumcloud/pleumcloud/internal/store"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("pleumcloud", version)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	st, err := store.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer st.Close()

	srv := server.New(cfg, st, version)
	addr := cfg.BindAddr()
	fmt.Printf(`
  ┌─────────────────────────────────────────────┐
  │                                             │
  │   PleumCloud — one drive for all your      │
  │   free cloud storage                        │
  │                                             │
  │   %s                                │
  │   version %s                              │
  │                                             │
  └─────────────────────────────────────────────┘

  Open %s in your browser.
  Data directory: %s
  Press Ctrl+C to stop.
`, "☁", version, addr, cfg.DataDir)

	if err := srv.ListenAndServe(addr); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

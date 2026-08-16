// PleumCloud — all your free cloud storage, one drive.
// Copyright (C) 2026 PleumCloud contributors. AGPL-3.0.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gachon-star-want/pleumcloud/internal/app"
	"github.com/gachon-star-want/pleumcloud/internal/browser"
	"github.com/gachon-star-want/pleumcloud/internal/ui"
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

	a, err := app.Start(app.Options{Version: version})
	if err != nil {
		log.Fatalf("start: %v", err)
	}

	// Start returns with the listener already bound, so the page is
	// guaranteed to load.
	fmt.Print(ui.Banner(version, a.URL, a.Cfg.DataDir))
	if a.Cfg.MultiUser {
		fmt.Println("  multi-user mode: registration open at /  (share http://<host>:" + fmt.Sprint(a.Cfg.Port) + ")")
	} else if a.Cfg.ServerMode {
		fmt.Println("  server mode: auth enabled — share http://<host>:" + fmt.Sprint(a.Cfg.Port))
	}
	fmt.Println("  Press Ctrl+C to stop.")

	if !a.Cfg.NoBrowser && !*noBrowser {
		if err := browser.Open(a.URL); err != nil {
			fmt.Printf("  (couldn't open a browser: %v - open %s manually)\n", err, a.URL)
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	if err := a.Close(); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

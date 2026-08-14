// PleumCloud — all your free cloud storage, one drive.
// Copyright (C) 2026 PleumCloud contributors. AGPL-3.0.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/pleumcloud/pleumcloud/internal/api"
	"github.com/pleumcloud/pleumcloud/internal/browser"
	"github.com/pleumcloud/pleumcloud/internal/config"
	"github.com/pleumcloud/pleumcloud/internal/index"
	"github.com/pleumcloud/pleumcloud/internal/oauthflow"
	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/secret"
	"github.com/pleumcloud/pleumcloud/internal/server"
	"github.com/pleumcloud/pleumcloud/internal/store"
	"github.com/pleumcloud/pleumcloud/internal/ui"

	// Connectors self-register into the provider registry.
	_ "github.com/pleumcloud/pleumcloud/internal/provider/bridge"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/drime"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/dropbox"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/gdrive"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/koofr"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/mybox"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/onedrive"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/pcloud"
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
	idx := index.New(st)

	a := api.New(st, secrets, oauth, idx, version)
	if err := a.LoadBYOCredentials(); err != nil {
		log.Fatalf("load credentials: %v", err)
	}

	// Background workers: keep the unified index fresh, drain transfers.
	go syncLoop(idx)
	go transferLoop(st, secrets)

	srv := server.New(cfg, a)
	addr := cfg.BindAddr()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}

	url := cfg.LocalURL()
	fmt.Print(ui.Banner(version, url, cfg.DataDir))
	fmt.Println("  Press Ctrl+C to stop.")

	// Open the default browser only after the listener is bound, so the
	// page is guaranteed to load.
	if !cfg.NoBrowser {
		if err := browser.Open(url); err != nil {
			fmt.Printf("  (couldn't open a browser: %v - open %s manually)\n", err, url)
		}
	}

	if err := srv.Serve(ln); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

// syncLoop refreshes every account's index: shortly after startup, then
// every 5 minutes.
func syncLoop(idx *index.Indexer) {
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		_ = idx.SyncAll(ctx, provider.Deps{}, provider.Build)
	}
	time.Sleep(3 * time.Second)
	run()
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		run()
	}
}

// transferLoop drains cross-cloud jobs sequentially, streaming source →
// server → destination so phones can drop off mid-transfer.
func transferLoop(st *store.Store, secrets secret.Store) {
	for {
		job, err := st.ClaimNextQueuedJob()
		if err != nil || job == nil {
			time.Sleep(2 * time.Second)
			continue
		}
		runTransfer(st, secrets, job)
	}
}

func runTransfer(st *store.Store, secrets secret.Store, job *store.JobRow) {
	fail := func(msg string) { _ = st.FinishJob(job.ID, "failed", msg) }

	srcRow, err := st.GetAccount(job.SrcAccount)
	if err != nil {
		fail("source account missing")
		return
	}
	dstRow, err := st.GetAccount(job.DstAccount)
	if err != nil {
		fail("destination account missing")
		return
	}
	srcConn, ok := provider.Build(srcRow.ProviderID, provider.Deps{Secrets: secrets})
	if !ok {
		fail("no source connector")
		return
	}
	dstConn, ok := provider.Build(dstRow.ProviderID, provider.Deps{Secrets: secrets})
	if !ok {
		fail("no destination connector")
		return
	}
	srcRef := provider.AccountRef{ID: srcRow.ID, ProviderID: srcRow.ProviderID, SecretRef: srcRow.SecretRef}
	dstRef := provider.AccountRef{ID: dstRow.ID, ProviderID: dstRow.ProviderID, SecretRef: dstRow.SecretRef}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
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
		_ = st.UpdateJobProgress(job.ID, done, total)
	}
	if _, err := dstConn.Upload(ctx, dstRef, "", job.FileName, rc, job.TotalBytes, progress); err != nil {
		fail(err.Error())
		return
	}
	_ = st.FinishJob(job.ID, "done", "")
}

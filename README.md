# PleumCloud ☁️

**All your free cloud storage, one drive.**

PleumCloud unifies the free tiers of Google Drive, OneDrive, Dropbox, Naver MyBox, Drime, pCloud, Koofr and any WebDAV service into a single drive you browse like one filesystem. Files stay whole on whichever cloud they live on — PleumCloud shows you where everything is and routes new uploads automatically.

## Highlights

- 🔗 **Connect with one click** — OAuth for Google Drive, OneDrive, Dropbox, pCloud; personal access tokens for Naver MyBox and Drime; WebDAV for everything else (Nextcloud, InfiniCLOUD, MagentaCloud, …)
- 🧠 **Smart placement** — new files go wherever you have the most free space by default; override per upload or set your own rules (e.g. *photos → Google, files > 1 GB → MyBox*)
- 🏷️ **Full visibility** — every file carries a badge showing which cloud it lives on, plus a per-cloud quota dashboard
- 🚚 **Server-side transfers** — copy or move files between clouds as background jobs that survive your browser closing
- 🔍 **Unified search** — one index across every connected account
- 📱 **Local-first, mobile-friendly** — a single binary with an embedded web UI and PWA; your phone is just a remote control

## Install (one line)

```bash
curl -fsSL https://pleumcloud.dev/install.sh | bash
```

Then open `http://localhost:7777`.

*Currently in early development — installer URL will go live with the first release. See [Roadmap](#roadmap).*

## Status (2026-08)

**M1–M4 core shipped** — 17 providers, unified drive, working end to end:

- **9 native connectors** (TDD, docs-pinned): Google Drive, OneDrive,
  Dropbox, Naver MyBox, Drime, pCloud, Koofr, WebDAV (incl. InfiniCLOUD)
- **8 experimental** via the rclone bridge: MEGA, Box, Yandex, HiDrive,
  Jottacloud, Filen, Internxt, Proton Drive
- Unified browsing with per-file cloud badges, cross-cloud FTS search,
  live quota dashboard, uploads with smart placement (+user rules),
  streaming downloads, share links, background cross-cloud transfers
- Secrets in the OS keychain; local SQLite index

See [docs/provider-decisions.md](docs/provider-decisions.md) for who is
supported and why, and [docs/oauth-setup.md](docs/oauth-setup.md) to set
up one-click OAuth (paste your app keys once).

## Roadmap

- **M5** Release: multi-OS binaries, Homebrew/Scoop, docs site
- **Post-MVP** Ubuntu server mode (self-hosted multi-device, cloud.pleum.ai),
  WebDAV mount, thumbnails/streaming polish, provider promotions from the
  rclone bridge to native, mobile push

## Development

```bash
# Backend (Go 1.26+)
go build -o pleumcloud .

# Frontend (Node 20+)
cd web && npm install && npm run build   # output embedded by the Go binary

# Run everything
make build && ./pleumcloud
```

## Why not iCloud / Samsung Cloud / TeraBox?

They have no official third-party APIs — connecting them would require unsupported reverse engineering that risks your accounts. PleumCloud only uses official APIs. See our docs for the full provider matrix.

## License

Copyright (C) 2026 PleumCloud contributors.

This program is free software: you can redistribute it and/or modify it under the terms of the [GNU Affero General Public License v3.0](LICENSE) as published by the Free Software Foundation.

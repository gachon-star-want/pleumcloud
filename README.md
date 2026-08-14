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

## Roadmap

- **M1** Skeleton: binary + embedded UI + database ✅ *(in progress)*
- **M2** Provider core: OAuth loopback flow + native connectors (Google Drive, OneDrive, Dropbox, pCloud, Koofr, MyBox, Drime, WebDAV) + experimental rclone bridge
- **M3** File management: browsing UI, uploads with auto-placement, quota dashboard
- **M4** Unified experience: rules engine, cross-cloud transfer queue, search, thumbnails, streaming, sharing
- **M5** Release: multi-OS binaries, Homebrew/Scoop, docs
- **Post-MVP** Ubuntu server mode (self-hosted multi-device), WebDAV mount, provider promotions

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

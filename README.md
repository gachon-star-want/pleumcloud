<p align="center">
  <img src="docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">All your free cloud storage, one drive.</h3>

<p align="center">
  <a href="https://github.com/gachon-star-want/pleumcloud/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/gachon-star-want/pleumcloud/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white&label=CI" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square&logo=gnu&logoColor=white" alt="License: AGPL-3.0"></a>
</p>

<p align="center">
  🌍 Read this in:
  <a href="readme/README.ko.md">한국어</a> ·
  <a href="readme/README.ja.md">日本語</a> ·
  <a href="readme/README.zh-CN.md">简体中文</a> ·
  <a href="readme/README.zh-TW.md">繁體中文</a> ·
  <a href="readme/README.es.md">Español</a> ·
  <a href="readme/README.fr.md">Français</a> ·
  <a href="readme/README.de.md">Deutsch</a> ·
  <a href="readme/README.pt-BR.md">Português (Brasil)</a>
</p>

---

PleumCloud is a local-first app that turns your scattered free cloud storage —
15 GB from Google, 30 GB from Naver MyBox, 5 GB from OneDrive, 20 GB from
Drime, 10 GB from pCloud… over 100 GB across six apps — into **one drive**.
Connect your accounts once, then browse, search, upload and move files as if
they all lived on a single disk. Every file carries a badge showing which
cloud it lives on.

Files are never re-hosted: each file lives whole on one cloud, and PleumCloud
keeps only a local index plus your credentials in the OS keychain. It ships
as a single ~18 MB binary with the UI embedded.

## Features

- 🔗 **One-click connect** — official OAuth for Google Drive, OneDrive,
  Dropbox and pCloud; access tokens for Naver MyBox, Drime, Koofr and
  MediaFire; WebDAV for everything else. Credentials live in your OS keychain.
- 🗂️ **One unified drive** — browse all clouds in a single view with per-file
  cloud badges, breadcrumbs and a live per-cloud quota dashboard.
- 🔍 **Instant search everywhere** — one keystroke searches every connected
  account through a local full-text index (SQLite FTS5).
- 🧠 **Smart placement** — uploads go wherever you have the most free space.
  Override per upload, or set rules (*"videos → Google, PDFs → MyBox,
  anything over 1 GB → pCloud"*) in the rules editor.
- 🚚 **Cross-cloud transfers** — copy or move files between clouds as
  background jobs that stream through your machine. Close the tab; the
  transfer keeps going.
- 👁 **Previews & streaming** — images, seekable video, audio and PDF
  previews, plus a gallery grid view with locally generated thumbnails.
- 🔗 **Share links, downloads, rename/move/copy/delete** — the everyday file
  ops, from one place.
- 🖥️ **Local-first** — single binary, no account, no telemetry. Need remote
  access? `PLEUMCLOUD_SERVER=1 PLEUMCLOUD_PASSWORD=… pleumcloud` guards it
  with Basic auth (pair with HTTPS on a NAS/VPS).
- 🧩 **18 providers** — 10 hand-built native connectors plus 8 more through
  the rclone bridge, all behind one interface.

## Supported clouds

| Provider | Free tier | Connect via | Status |
|---|---|---|---|
| Google Drive | 15 GB | OAuth 2.0 | ✅ Native |
| Microsoft OneDrive | 5 GB | OAuth 2.0 | ✅ Native |
| Dropbox | 2 GB | OAuth 2.0 | ✅ Native |
| Naver MyBox | 30 GB | Access token | ✅ Native |
| Drime | 20 GB | Access token | ✅ Native |
| pCloud | 10 GB | OAuth 2.0 | ✅ Native |
| Koofr | 10 GB | Email + token | ✅ Native |
| WebDAV (Nextcloud, ownCloud, MagentaCloud, …) | — | URL + login | ✅ Native |
| InfiniCLOUD | 20 GB | URL + login | ✅ Native |
| MediaFire | 10 GB | App credentials | ✅ Native |
| MEGA · Box · Yandex Disk · HiDrive · Jottacloud · Filen · Internxt · Proton Drive | 5–20 GB | [rclone](https://rclone.org) bridge | 🧪 Experimental |
| iCloud Drive · Samsung Cloud · TeraBox · Sync.com | — | — | ❌ No official API — see [why](docs/decisions.md) |

The full matrix of every service we evaluated, and the reasoning behind all
major design decisions, lives in [docs/decisions.md](docs/decisions.md).
The UI design system lives in [docs/design.md](docs/design.md).

## Install

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/scripts/install.sh | bash
```

Then run `pleumcloud` — your browser opens at `http://localhost:7777`.

**Docker (NAS/VPS):**

```bash
mkdir pleumcloud && cd pleumcloud && curl -fsSL -o docker-compose.yml   https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/docker-compose.yml
docker compose up -d          # → http://<host>:7777
```

**Other ways:**

```bash
# from source (Go 1.26+ and Node 20+)
git clone https://github.com/gachon-star-want/pleumcloud
cd pleumcloud && make build && ./pleumcloud

# Windows: download the .zip from Releases
```

**Running modes**

| Mode | How | Who it's for |
|---|---|---|
| Local | `pleumcloud` | One person, one machine — no auth, loopback only |
| Server | `PLEUMCLOUD_SERVER=1 PLEUMCLOUD_PASSWORD=… pleumcloud` | You, from anywhere — one shared password |
| Multi-user | `PLEUMCLOUD_MULTIUSER=1` (see compose) | Family/team — email sign-up, per-user data isolation |

**First OAuth connection?** Until the project's official OAuth apps finish
registration, grab a free app key once using
[docs/oauth-setup.md](docs/oauth-setup.md) (10 minutes, one-time per
machine) — after that every connect is a single click. Token-based clouds
(MyBox, Drime, Koofr) link you straight to their token pages.

## How it works

```
        ┌──────────────────────────────────────────────┐
        │                pleumcloud                    │
        │  ┌──────────┐  ┌─────────┐  ┌─────────────┐  │
 You ──▶│  │  Web UI  │  │ Indexer │  │  Placement  │  │
        │  └────┬─────┘  └────┬────┘  └──────┬──────┘  │
        │  ┌────┴────────────┴───────────────┴──────┐  │
        │        provider connectors × 18           │  │
        │  └───┬──────┬──────┬──────┬──────┬───────┘  │
        └──────┼──────┼──────┼──────┼──────┼───────────┘
               ▼      ▼      ▼      ▼      ▼
           Google   One   Dropbox  MyBox   WebDAV …
           (files stay on YOUR clouds — PleumCloud is the control plane)
```

PleumCloud is the control plane, not a host. The indexer keeps a local
SQLite/FTS5 catalog of names, sizes and dates (synced incrementally via each
provider's change feed), the placement engine decides where new files go,
and cross-cloud transfers stream source → your machine → destination using
each provider's resumable upload protocol.

## Development

Requires Go 1.26+ and Node 20+.

```bash
make dev    # go run + Vite dev server (hot reload)
make build  # frontend bundle → embedded Go binary
make test   # go test ./...
```

The backend lives in `internal/` (api, provider, index, placement, …) and
the React app in `web/`.

## Contributing

Issues and PRs are welcome — including **new connectors**, UI polish, docs
and translations. Each connector lives in `internal/provider/<name>/` behind
a single interface with mock tests; start with
[CONTRIBUTING.md](CONTRIBUTING.md).

## Privacy

No telemetry, no analytics, no accounts. Tokens stay in your OS keychain;
the index stays in `~/.pleumcloud/`. Details:
[privacy policy](web/public/privacy.html).

## License

PleumCloud is free software under the **GNU AGPL-3.0** — use, study, modify
and self-host it freely. If you offer it as a network service, you must
share your modifications under the same license.

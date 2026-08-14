<p align="center">
  <img src="docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">All your free cloud storage, one drive.</h3>

<p align="center">
  <a href="https://github.com/gachon-star-want/pleumcloud/releases"><img src="https://img.shields.io/github/v/release/gachon-star-want/pleumcloud?style=flat-square&logo=github&color=%232563eb" alt="Release"></a>
  <a href="https://github.com/gachon-star-want/pleumcloud/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/gachon-star-want/pleumcloud/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white" alt="CI"></a>
  <a href="https://goreportcard.com/report/github.com/gachon-star-want/pleumcloud"><img src="https://goreportcard.com/badge/github.com/gachon-star-want/pleumcloud?style=flat-square" alt="Go Report"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square" alt="License: AGPL-3.0"></a>
  <a href="#"><img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey?style=flat-square&logo=windows" alt="Platforms"></a>
  <a href="https://github.com/gachon-star-want/pleumcloud/pulls"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square" alt="PRs welcome"></a>
</p>

<p align="center"><b>
  <a href="#install">Install</a> ·
  <a href="#the-problem">Why</a> ·
  <a href="#supported-clouds">Clouds</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="docs/oauth-setup.md">OAuth setup</a> ·
  <a href="#contributing">Contribute</a>
</b></p>

<p align="center">
  🌍 Read this in:
  <a href="readme/README.ko.md">한국어</a> ·
  <a href="readme/README.ja.md">日本語</a> ·
  <a href="readme/README.zh.md">简体中文</a> ·
  <a href="readme/README.es.md">Español</a> ·
  <a href="readme/README.fr.md">Français</a> ·
  <a href="readme/README.de.md">Deutsch</a> ·
  <a href="readme/README.pt-BR.md">Português</a> ·
  <a href="readme/README.ru.md">Русский</a> ·
  <a href="readme/README.hi.md">हिन्दी</a> ·
  <a href="readme/README.id.md">Indonesia</a> ·
  <a href="readme/README.it.md">Italiano</a> ·
  <a href="readme/README.vi.md">Tiếng Việt</a>
</p>

---

## The problem

You already have **plenty of free cloud storage** — 15 GB from Google, 30 GB
from Naver MyBox, 5 GB from OneDrive, 20 GB from Drime, 10 GB from pCloud,
20 GB from InfiniCLOUD… over **100 GB, scattered across six apps**, each
with its own login, its own upload button, and its own idea of "search".

PleumCloud turns that pile into **one drive**. Connect your accounts once,
then browse, search, upload and move files as if they all lived on a single
disk — every file carries a badge showing which cloud it lives on.

> Born in August 2026, the week Naver finally opened the
> [MyBox Open API](https://developers.mybox.naver.com) after 13 years —
> the last big Korean cloud had no reason left to stay closed, and free
> storage everywhere finally had a reason to work together.

## ✨ Features

- 🔗 **One-click connect** — official OAuth for Google Drive, OneDrive,
  Dropbox and pCloud; access tokens for Naver MyBox, Drime and Koofr;
  WebDAV for everything else. Credentials live in your OS keychain.
- 🗂️ **One unified drive** — browse all clouds in a single view with
  per-file cloud badges, breadcrumbs and a live per-cloud quota dashboard.
- 🔍 **Instant search everywhere** — one keystroke searches every
  connected account through a local full-text index.
- 🧠 **Smart placement** — uploads automatically go wherever you have the
  most free space. Override per upload, or set rules
  (*"videos → Google, PDFs → MyBox, anything over 1 GB → pCloud"*).
- 🚚 **Cross-cloud transfers** — copy or move files between clouds as
  background jobs that stream through your machine. Close the tab; the
  transfer keeps going.
- 🔗 **Share links, downloads, rename/move/copy/delete** — the everyday
  file ops, from one place.
- 🖥️ **Local-first** — a single ~18 MB binary with the UI embedded. No
  account, no telemetry, no server required (self-hosted server mode is on
  the roadmap).
- 🧩 **17 providers** — 9 hand-built native connectors plus 8 more through
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
| MEGA · Box · Yandex Disk · HiDrive · Jottacloud · Filen · Internxt · Proton Drive | 5–20 GB | [rclone](https://rclone.org) bridge | 🧪 Experimental |
| iCloud Drive · Samsung Cloud · TeraBox · Sync.com | — | — | ❌ No official API — see [why](docs/provider-decisions.md) |

MediaFire (official REST API) is next in the pipeline. The full matrix,
including every service we evaluated and rejected, lives in
[docs/provider-decisions.md](docs/provider-decisions.md).

## Install

**macOS / Linux:**

```bash
curl -fsSL https://pleumcloud.dev/install.sh | bash
```

Then run `pleumcloud` — your browser opens at `http://localhost:7777`.

**Other ways:**

```bash
# from source (Go 1.26+ and Node 20+)
git clone https://github.com/gachon-star-want/pleumcloud
cd pleumcloud && make build && ./pleumcloud

# Windows: download the .zip from Releases
```

**First OAuth connection?** Grab a free app key once using
[docs/oauth-setup.md](docs/oauth-setup.md) (10 minutes, one-time per
machine) — after that every connect is a single click.

## How it works

```
        ┌──────────────────────────────────────────────┐
        │                pleumcloud                    │
        │  ┌──────────┐  ┌─────────┐  ┌─────────────┐  │
 You ──▶│  │  Web UI  │  │ Indexer │  │  Placement  │  │
        │  └────┬─────┘  └────┬────┘  └──────┬──────┘  │
        │  ┌────┴────────────┴───────────────┴──────┐  │
        │  │        provider connectors × 17        │  │
        │  └───┬──────┬──────┬──────┬──────┬───────┘  │
        └──────┼──────┼──────┼──────┼──────┼───────────┘
               ▼      ▼      ▼      ▼      ▼
           Google   One   Dropbox  MyBox   WebDAV …
           (files stay on YOUR clouds — PleumCloud is the control plane)
```

Files are never re-hosted: each file lives whole on one cloud, and
PleumCloud keeps only a local index (names, sizes, dates) plus your
credentials in the OS keychain. Cross-cloud transfers stream
source → your machine → destination using each provider's resumable
upload protocol.

## Why not…?

- **MultCloud / cloudHQ** — closed SaaS; your cloud tokens live on their
  servers, and the free tier is ad-limited. PleumCloud is open source and
  runs on *your* machine.
- **rclone alone** — brilliant CLI, but no unified UI, no unified search
  index, no placement rules. (We love it so much we embed it as the
  bridge for long-tail providers.)
- **iCloud / Samsung Cloud** — no official third-party API exists;
  reverse-engineering them risks your accounts, so we don't.

## Roadmap

- [x] M1 — single binary + embedded UI
- [x] M2 — 9 native connectors + rclone bridge (OAuth, keychain, TDD)
- [x] M3 — unified browsing, search, quota dashboard
- [x] M4 — smart placement + rules, cross-cloud transfer queue, sharing
- [ ] M5 — thumbnails & gallery, video streaming, rules editor UI
- [ ] Self-hosted server mode (accounts, HTTPS, Docker, NAS images)
- [ ] Hosted service for everyone else

## Contributing

Issues and PRs are welcome — including **new connectors** (each one lives
in `internal/provider/<name>/` behind a single interface with mock tests),
UI polish, docs and translations. See
[CONTRIBUTING.md](CONTRIBUTING.md) to get started.

## Privacy

No telemetry, no analytics, no accounts. Tokens stay in your OS keychain;
the index stays in `~/.pleumcloud/`. Details: [privacy policy](/privacy).

## License

PleumCloud is free software under the **GNU AGPL-3.0** — you can use,
study, modify and self-host it freely. If you offer it as a network
service, you must share your modifications under the same license.

## Acknowledgments

- [rclone](https://rclone.org) — the bridge for long-tail providers, and a
  constant source of protocol wisdom.
- [go-koofrclient](https://github.com/koofr/go-koofrclient) and the
  Naver MyBox team — reference implementations we pinned our connectors
  against.
- Every cloud provider that chose to open an API.

---

<p align="center">☁️ <i>Built by <a href="https://github.com/gachon-star-want">Discover_it</a> and contributors — because 100 GB of free storage deserves one home.</i></p>

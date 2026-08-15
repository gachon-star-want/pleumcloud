# Design decisions

This document records *why* PleumCloud is shaped the way it is — the
reasoning behind the architecture, the provider lineup, and the contribution
model — so contributors can understand the intent (and challenge it, via
issues, when the trade-offs change). Each entry is decision → why →
rejected alternatives.

## Architecture

### D1 — Local-first single binary, not a hosted service

**Decision.** PleumCloud ships as one ~18 MB Go binary with the web UI
embedded (`go:embed`), serving `localhost:7777`. No server, no accounts.

**Why.** The app's whole value is holding *your* cloud tokens. Asking users
to hand those to a third-party server — as the commercial cloud-to-cloud
services do — would break the trust the project is built on, and add
hosting costs. A single binary also means zero-install friction.

**Rejected.** Hosted SaaS (token custody, ad-limited free tiers); Electron
app (bundle weight); separate backend + hosted frontend for v1 (deployment
friction).

### D2 — Go backend, React frontend, pure-Go SQLite

**Decision.** Go backend, React 19 + Vite UI, both packed into one binary.
SQLite via `modernc.org/sqlite` (pure Go, no CGO) with FTS5 for search.

**Why.** `go build` cross-compiles the whole app — database driver included
— to every platform from one machine; CGO would break that. The embed keeps
frontend and backend as one artifact, and contributors run everything with
`make dev`.

**Rejected.** Node backend (runtime footprint); Rust (thinner OAuth/keyring
ecosystem at design time); CGO SQLite (cross-compile pain).

### D3 — Control plane only: files stay whole on one cloud

**Decision.** PleumCloud never re-hosts or splits files. It keeps a local
metadata index (names, sizes, dates) and orchestrates operations against
provider APIs.

**Why.** Re-hosting means storage costs and ToS gray zones; splitting files
across clouds would make them unrecoverable without PleumCloud. Keeping
files whole means every file remains a normal file on a normal cloud —
with or without PleumCloud.

**Rejected.** Deduplicated storage pool (unrecoverable without this tool);
server-side app-to-app copies (would require holding everyone's tokens).

### D4 — Native connectors first, rclone bridge for the long tail

**Decision.** 10 providers are hand-built in `internal/provider/<name>/`;
8 more ride on an optional [rclone](https://rclone.org) sidecar and are
labeled experimental. Bridge providers graduate to native as demand
justifies the work.

**Why.** Native connectors get the full experience — incremental change
feeds, resumable uploads, quota, thumbnails, one-click auth. But writing
protocol code for every cloud on earth doesn't scale for a young project;
rclone already did. The bridge is optional: the core experience works
without it.

**Rejected.** rclone-only (no unified search index, no placement rules, no
embedded auth UI).

### D5 — OAuth with no embedded secrets: bring your own app

**Decision.** The OAuth flow ships with zero embedded client secrets.
Users register their own (free) app keys once —
[docs/oauth-setup.md](oauth-setup.md) walks through each provider's
console.

**Why.** OAuth verification for a small open-source project is slow and
expensive (e.g. Google's verification/CASA process caps unverified apps at
100 users). With BYO keys, every user is their own app owner: no shared
quota, no verification wall, and the repository holds nothing worth
stealing.

**Rejected.** Hardcoded shared app credentials (verification walls, secret
leakage risk); mandatory manual token pasting for OAuth providers (bad
first-run experience — BYO keys still give one-click connects afterwards).

### D6 — Credentials in the OS keychain, metadata in SQLite

**Decision.** Access/refresh tokens go into the OS credential store
(Keychain, Windows Credential Manager, Linux Secret Service) via
`zalando/go-keyring`. The local database holds filenames, sizes and dates
only — never file contents, never tokens.

**Why.** A stolen `~/.pleumcloud/` directory is worthless: no secrets, no
files — just a list of names. The same promise is why there is no telemetry
and no analytics.

### D7 — Placement: most-free-space by default, user rules on top

**Decision.** Uploads default to the cloud with the most free space. A
first-match-wins rules engine (editable in the UI: *"videos → Google, PDFs
→ MyBox, anything over 1 GB → pCloud"*) overrides the default; no rule
matching falls back to most-free-space.

**Why.** The whole point of pooling free tiers is using them, and
most-free-space maximizes headroom without user input. Rules capture intent
the heuristic can't see.

**Rejected.** Round-robin spread (wastes the biggest tiers); manual choice
only (misses the point of pooling).

### D8 — Transfers stream through the user's machine

**Decision.** Cross-cloud copies stream source → the machine running
PleumCloud → destination, as background jobs that survive the browser tab
closing.

**Why.** The only party allowed to hold both clouds' credentials is the
user's own machine. Streaming with each provider's resumable upload
protocol keeps transfers restartable and disk usage near zero.

**Rejected.** Server-side transfer relay (token custody); temp-file copies
(disk pressure for large files).

### D9 — AGPL-3.0

**Decision.** GNU AGPL-3.0.

**Why.** PleumCloud's promise is "your tokens never leave your machine."
AGPL requires anyone who turns a fork into a hosted service to share their
modifications — the license encodes the project's core promise.

### D10 — One Provider interface, tests written first

**Decision.** Every connector lives in `internal/provider/<name>/` behind a
single `Provider` interface, implemented against provider docs pinned in
the package, with mock-based tests written before the client code.
[CONTRIBUTING.md](../CONTRIBUTING.md) documents the recipe.

**Why.** Connectors are where most contributions will land. A single
interface + docs-pinned mock tests keep behavior uniform across
contributors without requiring live accounts to run CI.

## Provider matrix

Inclusion criteria: **official third-party API + meaningful free tier +
service health.** Reviewed against the
[namu.wiki cloud storage list](https://namu.wiki/w/클라우드%20스토리지/목록)
(2026-08-14) plus our own API research. Naver MyBox opened its official
Open API on 2026-08-11 after 13 years closed — the event that made this
project possible.

### Supported

| Provider | Tier | Auth | Free |
|---|---|---|---|
| Google Drive, OneDrive, Dropbox | native | OAuth2 | 15/5/2 GB |
| Naver MyBox | native | PAT (Open API launched 2026-08-11) | 30 GB |
| Drime | native | PAT (public API, docs.drime.cloud) | 20 GB |
| pCloud | native | OAuth2 | 10 GB |
| Koofr | native | PAT | 10 GB |
| WebDAV (generic: Nextcloud, ownCloud, Pydio, Seafile, InfiniCLOUD, MagentaCloud, mailbox.org, kDrive paid, …) | native | WebDAV | varies |
| MediaFire | native | session token (official REST API, app registration) | 10 GB |
| MEGA, Box, Yandex Disk, HiDrive, Jottacloud, Filen, Internxt | experimental (rclone bridge) | varies | 5–20 GB |
| Proton Drive | experimental — promote the day the official API lands | — | 5 GB |

### Evaluated and excluded

| Service | Verdict | Why |
|---|---|---|
| iCloud Drive | excluded | No official API; rclone's unofficial backend needs re-auth every 30 days |
| Samsung Cloud / T cloud / Kakao Talk Cloud / KT·LG U+ clouds | excluded | No public API; telecom consumer clouds all shut down (2018–2021) |
| Cloudike (클라우다이크) | excluded | B2B/white-label SaaS only, no consumer free tier (namu s-3.2) |
| Hancom Docs, Polaris Office | excluded | Office-suite clouds, no public file-storage API |
| Kios Cloud (키오스클라우드) | excluded | Niche, no API, chronic stability issues |
| Naver Works Drive | excluded | Real OAuth API but org-only (admin app registration) — revisit for a business edition |
| Baidu Netdisk, 115, Aliyun Drive, Quark, Weiyun | excluded | Approval walls / enterprise-only onboarding / ToS risk |
| TeraBox, PikPak | excluded | Unofficial reverse-engineered APIs only, account-ban risk |
| Sync.com, Icedrive, Degoo, ASUS WebStorage (xDrive) | excluded | No public API (Degoo app-only; Icedrive WebDAV being phased out; xDrive never had one) |
| SugarSync | excluded | Fully paid (no free tier) |
| OpenDrive | excluded | API alive but service stagnant; ToS allows terminating Basic accounts at will |
| hubiC, Amazon Drive, @nifty | dead | Service discontinued |
| Google Photos, Flickr | out of scope (v1) | Photo libraries, not drives — possible future feature |
| Sia, Storj, Backblaze B2, R2 | out of scope (v1) | Object storage (S3-style), different product category |
| GitHub | out of scope | Repositories are not file storage (ToS) |

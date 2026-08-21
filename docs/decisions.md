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

### D5 — Official OAuth apps first, BYO fallback (revised 2026-08-17)

**Decision.** PleumCloud ships the project's official OAuth apps as public
client IDs/secrets (the rclone model) so connecting a cloud is one click
with zero setup: the provider's own sign-in page opens, you approve, done.
Users may still bring their own app key per provider (it wins over the
built-in one), and self-host/server deployments inject their own via
`PLEUMCLOUD_OAUTH_<PROVIDER>_CLIENT_ID`/`_CLIENT_SECRET`. Token-only
providers (MyBox, Drime, Koofr) deep-link to the provider's token-creation
page — PleumCloud never asks for a cloud account password.

**Why.** The original no-secrets policy put a developer-console chore in
front of every new user and the paste-key modal read as "this app wants my
credentials" — provider-site login is the expected UX (GitHub↔Claude
style). A local-first binary cannot keep a client secret confidential
anyway; the real protections are PKCE (S256, enabled for
gdrive/onedrive/dropbox) and the redirect URIs each registered app is
locked to.

**Rejected.** Zero embedded secrets, BYO only (original 2026-08-14
decision — chosen to dodge verification walls like Google's testing-mode
100-user cap and Dropbox's dev-app limits; those caps are accepted now and
lifted per-provider as approvals land); mandatory manual token pasting for
OAuth providers.

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

### D11 — One codebase, three product forms, sequential B2C launch

*(Decided 2026-08-17. Amends D1: local-first stays the default and the
trust anchor; server/hosted become opt-in forms on top of the same core.)*

**Decision.** One codebase ships three forms: **local** (default, as
today), **server** (`_SERVER=1`, self-hosted OSS), and **hosted**
(the `_MULTIUSER=1` mode operated by us, gated on per-user secret
encryption). B2C launches sequentially: desktop app (Wails v2 shell
around the local core, with a license key + freemium gates) first,
hosted second. `PleumCloud_PaaS/` — a separate hosted codebase sharing
zero code with the core — is archived: kept, but no new investment.

**Why.** Two parallel codebases is the classic way a solo project ships
neither. The multiuser mode already lives in the core, so hosted needs
hardening, not a rewrite. Market timing: odrive killed its free tier
(2026-03-31) leaving paying refugees looking for alternatives; MultCloud
proves hosted aggregation monetizes. Desktop-first also earns the trust
story (tokens stay local) that hosted then rides on as a premium.

**Rejected.** Keeping the separate PaaS codebase (zero code reuse,
duplicate product); launching both forms simultaneously (solo bandwidth);
hosted-first (weaker trust story, token-custody liability before any
paying users exist).

### D12 — Two-track open source: web app + desktop app, revenue via hosted

*(Decided 2026-08-17. Amends D11's desktop monetization: both tracks
ship fully open source. Reconfirmed 2026-08-21: the desktop beta ships
first with zero monetization work in it; the hosted tier stays deferred
— later, not abandoned.)*

**Decision.** Both product forms — the web app (local binary +
self-hosted server mode) and the desktop app (Wails shell around the
core) — ship as open source with no feature gates in the repository.
Revenue comes from the hosted tier (and possible future services), not
from locking OSS features. The license-key/freemium gates from D11's
desktop phase are dropped.

**Why.** Gates inside an AGPL repo get forked out and cost trust.
Bitwarden and Immich show the working model: free, open clients build
adoption, and the hosted service converts convenience demand. A fully
open desktop app also strengthens the tokens-stay-local story that the
hosted tier rides on — and it simplifies the roadmap: the desktop
phase becomes shell + packaging only.

**Rejected.** Freemium gates inside OSS code (forked out, trust cost);
a closed desktop edition (contradicts the AGPL core and re-creates the
two-codebase problem D11 closed).

### D13 — Desktop shell: loopback embedding, lean MVP, unsigned first

*(Decided 2026-08-17. Implements D12's desktop phase.)*

**Decision.** Three calls for the Wails v2 shell:

- **(a) Architecture — loopback.** The shell calls `app.Start` with
  `Port 7777` on `127.0.0.1` and the webview loads that URL. The web
  app's runtime path (OAuth `r.Host` redirect URIs, cookies, streaming
  downloads) stays byte-identical to the web binary. OAuth consent pages
  open in the system browser (Google blocks embedded webviews).
- **(b) MVP scope.** Single window + single-instance lock, close
  confirmation while transfers are active, app name/icon, manual update
  banner (GitHub Releases comparison). Tray, OS auto-start, and
  auto-update are deferred to v0.4+ (Wails v2 has no native tray).
- **(c) Distribution — unsigned first.** Ship unsigned (dmg + NSIS),
  mitigated by a Homebrew cask (curl downloads carry no quarantine
  attribute) and install docs. The release pipeline is signing-ready:
  notarization/code-signing activates when secrets are registered.
  Revisit before the October launch.

**Why.** The alternative — mounting `server.Handler()` on Wails' asset
handler — breaks `r.Host`-based OAuth redirects, needs a separate
loopback listener for callbacks anyway, and forks the runtime path into
desktop-only variants. Loopback reuses everything (JupyterLab Desktop
precedent). Signing costs money before any users exist; unsigned plus
brew gets 90% of the UX for free and the pipeline can turn signing on
later without a rebuild.

**Rejected.** Wails asset-handler mounting (OAuth/callback breakage,
dual code paths); custom URL scheme (would force re-registering every
provider console); tray/auto-start in v1 (v2 unsupported, third-party
fork risk); buying certificates now (revisited pre-launch).

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

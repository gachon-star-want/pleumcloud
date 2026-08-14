# Provider decisions

Why each cloud storage service is or isn't in PleumCloud. Reviewed against
the [namu.wiki cloud storage list](https://namu.wiki/w/클라우드%20스토리지/목록)
(2026-08-14) plus our own API research. Inclusion criteria: **official
third-party API + meaningful free tier + service health.**

## Supported

| Provider | Tier | Auth | Free |
|---|---|---|---|
| Google Drive, OneDrive, Dropbox | native | OAuth2 | 15/5/2 GB |
| Naver MyBox | native | PAT (Open API launched 2026-08-11) | 30 GB |
| Drime | native | PAT (public API, docs.drime.cloud) | 20 GB |
| pCloud | native | OAuth2 | 10 GB |
| Koofr | native | PAT | 10 GB |
| WebDAV (generic: Nextcloud, ownCloud, Pydio, Seafile, InfiniCLOUD, MagentaCloud, mailbox.org, kDrive paid, …) | native | WebDAV | varies |
| MediaFire | experimental (official REST API, app registration) | session token | 10 GB |
| MEGA, Box, Yandex Disk, HiDrive, Jottacloud, Filen, Internxt | experimental (rclone bridge) | varies | 5–20 GB |
| Proton Drive | experimental — promote the day the official CLI/API lands | — | 5 GB |

## Evaluated and excluded

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

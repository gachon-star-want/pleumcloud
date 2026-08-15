<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">Dein gesamter kostenloser Cloud-Speicher — ein Laufwerk.</h3>

<p align="center">
  <a href="https://github.com/gachon-star-want/pleumcloud/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/gachon-star-want/pleumcloud/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white&label=CI" alt="CI"></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square&logo=gnu&logoColor=white" alt="Lizenz: AGPL-3.0"></a>
</p>

<p align="center">
  🌍 [English](../README.md) · [한국어](README.ko.md) · [日本語](README.ja.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · [Español](README.es.md) · [Français](README.fr.md) · Deutsch · [Português (Brasil)](README.pt-BR.md)
</p>

---

PleumCloud ist eine Local-first-Anwendung, die deinen verstreuten kostenlosen Cloud-Speicher — 15 GB von Google, 30 GB von Naver MyBox, 5 GB von OneDrive, 20 GB von Drime, 10 GB von pCloud… über 100 GB verteilt auf sechs Apps — in **ein Laufwerk** verwandelt. Verbinde deine Konten einmal, dann durchstöbere, suche, lade hoch und verschiebe Dateien, als lebten sie alle auf einer einzigen Festplatte. Jede Datei trägt ein Abzeichen, das zeigt, in welcher Cloud sie liegt.

Dateien werden nie neu gehostet: Jede Datei liegt vollständig in einer Cloud, und PleumCloud speichert nur einen lokalen Index plus deine Zugangsdaten im Schlüsselbund des Systems. Es wird als ein einzelnes ~18-MB-Binary mit eingebetteter UI ausgeliefert.

## Funktionen

- 🔗 **Verbindung mit einem Klick** — offizielles OAuth für Google Drive, OneDrive, Dropbox und pCloud; Zugangstoken für Naver MyBox, Drime, Koofr und MediaFire; WebDAV für alles andere. Zugangsdaten leben im Schlüsselbund deines Systems
- 🗂️ **Ein einheitliches Laufwerk** — durchstöbere alle Clouds in einer einzigen Ansicht mit Cloud-Abzeichen pro Datei, Brotkrumen-Navigation und einem Live-Dashboard der Cloud-Speicherplätze
- 🔍 **Sofortige Suche überall** — ein einziger Tastenanschlag durchsucht alle verbundenen Konten über einen lokalen Volltextindex (SQLite FTS5)
- 🧠 **Smarte Platzierung** — Uploads landen dort, wo du den meisten freien Speicher hast. Pro Upload übersteuern oder Regeln festlegen (*„Videos → Google, PDFs → MyBox, alles über 1 GB → pCloud"*) im Regel-Editor
- 🚚 **Cloud-übergreifende Übertragungen** — Dateien zwischen Clouds kopieren oder verschieben als Hintergrundaufträge, die durch deine Maschine streamen. Tab schließen; die Übertragung läuft weiter
- 👁 **Vorschauen & Streaming** — Bilder, spulbares Video, Audio und PDF plus Galerie-Rasteransicht mit lokal erzeugten Vorschaubildern
- 🔗 **Freigabelinks, Downloads, Umbenennen/Verschieben/Kopieren/Löschen** — die alltäglichen Dateioperationen, an einem Ort
- 🖥️ **Local-first** — ein einzelnes Binary, ohne Konto, ohne Telemetrie. Fernzugriff nötig? `PLEUMCLOUD_SERVER=1 PLEUMCLOUD_PASSWORD=… pleumcloud` schützt es mit Basic Auth (kombiniere es mit HTTPS auf einem NAS/VPS)
- 🧩 **18 Anbieter** — 10 handgebaute native Konnektoren plus 8 weitere über die rclone-Bridge, alle hinter einer Schnittstelle

## Unterstützte Clouds

| Anbieter | Gratis-Stufe | Verbindung | Status |
|---|---|---|---|
| Google Drive | 15 GB | OAuth 2.0 | ✅ Nativ |
| Microsoft OneDrive | 5 GB | OAuth 2.0 | ✅ Nativ |
| Dropbox | 2 GB | OAuth 2.0 | ✅ Nativ |
| Naver MyBox | 30 GB | Zugangstoken | ✅ Nativ |
| Drime | 20 GB | Zugangstoken | ✅ Nativ |
| pCloud | 10 GB | OAuth 2.0 | ✅ Nativ |
| Koofr | 10 GB | E-Mail + Token | ✅ Nativ |
| WebDAV (Nextcloud, ownCloud, MagentaCloud, …) | — | URL + Login | ✅ Nativ |
| InfiniCLOUD | 20 GB | URL + Login | ✅ Nativ |
| MediaFire | 10 GB | App-Credentials | ✅ Nativ |
| MEGA · Box · Yandex Disk · HiDrive · Jottacloud · Filen · Internxt · Proton Drive | 5–20 GB | [rclone](https://rclone.org)-Bridge | 🧪 Experimentell |
| iCloud Drive · Samsung Cloud · TeraBox · Sync.com | — | — | ❌ Keine offizielle API — [warum](../docs/decisions.md) |

Die vollständige Matrix aller evaluierten Dienste und die Begründung aller größeren Design-Entscheidungen liegt in [docs/decisions.md](../docs/decisions.md).

## Installation

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/scripts/install.sh | bash
```

Führe danach `pleumcloud` aus — dein Browser öffnet `http://localhost:7777`.

**Andere Wege:**

```bash
# aus dem Quellcode (Go 1.26+ und Node 20+)
git clone https://github.com/gachon-star-want/pleumcloud
cd pleumcloud && make build && ./pleumcloud

# Windows: das .zip aus den Releases laden
```

**Erste OAuth-Verbindung?** Hol dir einmalig einen kostenlosen App-Schlüssel per [docs/oauth-setup.md](../docs/oauth-setup.md) (10 Minuten, einmal pro Maschine) — danach ist jede Verbindung ein einziger Klick.

## Funktionsweise

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

PleumCloud ist die Kontrollebene, nicht der Host. Der Indexer hält über den Änderungs-Feed jedes Anbieters inkrementell synchron ein lokales SQLite/FTS5-Verzeichnis von Namen, Größen und Daten; die Platzierungs-Engine entscheidet, wohin neue Dateien gehen; und Cloud-übergreifende Übertragungen streamen Quelle → deine Maschine → Ziel über das fortsetzbare Upload-Protokoll jedes Anbieters.

## Entwicklung

Benötigt Go 1.26+ und Node 20+.

```bash
make dev    # go run + Vite-Dev-Server (Hot Reload)
make build  # Frontend-Bundle → eingebettetes Go-Binary
make test   # go test ./...
```

Das Backend liegt in `internal/` (api, provider, index, placement, …), die React-App in `web/`.

## Mitwirken

Issues und PRs sind willkommen — inklusive **neuer Konnektoren**, UI-Verbesserungen, Docs und Übersetzungen. Jeder Konnektor liegt in `internal/provider/<name>/` hinter einer einzigen Schnittstelle mit Mock-Tests; beginne mit [CONTRIBUTING.md](../CONTRIBUTING.md).

## Datenschutz

Keine Telemetrie, keine Analysen, keine Konten. Tokens bleiben im Schlüsselbund deines Systems; der Index bleibt in `~/.pleumcloud/`. Details: [Datenschutzerklärung](../web/public/privacy.html).

## Lizenz

PleumCloud ist freie Software unter der **GNU AGPL-3.0** — nutze, studiere, verändere und self-hoste sie frei. Bietest du sie als Netzwerkdienst an, musst du deine Änderungen unter derselben Lizenz teilen.

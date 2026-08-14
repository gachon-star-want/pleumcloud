<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">All dein kostenlosen Cloud-Speicher, ein Laufwerk.</h3>

> 🌍 [English](../README.md) · Deutsch

Google 15 GB, OneDrive 5 GB, pCloud 10 GB… du hast bereits **über 100 GB gratis**, verteilt auf sechs Apps — jede mit eigenem Login, eigenem Upload-Button und eigener Suche.

**PleumCloud macht daraus ein einziges Laufwerk.** Konten einmal verbinden, dann alle Dateien in einer Ansicht durchsuchen, durchsuchen lassen, hochladen und verschieben — jede Datei trägt ein Badge, das zeigt, in welcher Cloud sie lebt.

> Entstanden im August 2026, in der Woche, in der Naver nach 13 Jahren endlich die
> [MyBox Open API](https://developers.mybox.naver.com) öffnete.

## ✨ Funktionen

- 🔗 **Ein-Klick-Verbindung** — offizielles OAuth (Google, OneDrive, Dropbox, pCloud); Zugriffs-Tokens (MyBox, Drime, Koofr); WebDAV für den Rest. Zugangsdaten bleiben im OS-Schlüsselbund
- 🗂️ **Ein vereinheitlichtes Laufwerk** — eine Ansicht mit Cloud-Badges + Live-Speicher-Dashboard
- 🔍 **Globale Suche** — ein Tastenanschlag durchsucht alle verbundenen Konten
- 🧠 **Smarte Platzierung** — Uploads landen im Cloud mit dem meisten freien Platz; Regeln konfigurierbar
- 🚚 **Cloud-übergreifende Übertragungen** — Hintergrundjobs, die weiterlaufen, während du den Tab schließt
- 🖥️ **Local-first** — ein einziges Binary (~18 MB) mit eingebetteter UI, ohne Server
- 🧩 **17 Anbieter** — 9 native Konnektoren + 8 über die rclone-Bridge

## Installation (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/scripts/install.sh | bash
```

`pleumcloud` starten — der Browser öffnet `http://localhost:7777`.

## Lizenz

**GNU AGPL-3.0**

---

<p align="center">☁️ <i>Erstellt von [Discover_it](https://github.com/gachon-star-want) und Beitragenden.</i></p>

<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">把散落的免費雲端空間，合併成一個硬碟。</h3>

<p align="center">
  <a href="https://github.com/gachon-star-want/pleumcloud/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/gachon-star-want/pleumcloud/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white&label=CI" alt="CI"></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square&logo=gnu&logoColor=white" alt="授權條款：AGPL-3.0"></a>
</p>

<p align="center">
  🌍 [English](../README.md) · [한국어](README.ko.md) · [日本語](README.ja.md) · [简体中文](README.zh-CN.md) · 繁體中文 · [Español](README.es.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Português (Brasil)](README.pt-BR.md)
</p>

---

PleumCloud 是一款本機優先（local-first）的應用程式，把你散落各處的免費雲端空間 —— Google 15GB、Naver MyBox 30GB、OneDrive 5GB、Drime 20GB、pCloud 10GB…… 分散在六個應用裡、超過 100GB 的容量 —— 合併成**一個硬碟**。只需連結一次帳號，就能像使用單一磁碟一樣瀏覽、搜尋、上傳和移動所有雲端的檔案，每個檔案都會標示所屬雲端的徽章。

檔案絕不會被重新託管：每個檔案完整保存在原本的雲端上，PleumCloud 只保留本機索引和系統鑰匙圈中的憑證。它以單一約 18MB、內嵌 UI 的二進位檔發佈。

## 功能特色

- 🔗 **一鍵連結** — Google Drive、OneDrive、Dropbox、pCloud 使用官方 OAuth；Naver MyBox、Drime、Koofr、MediaFire 使用存取權杖；其餘使用 WebDAV。憑證保存在系統鑰匙圈
- 🗂️ **統一硬碟** — 在單一檢視中瀏覽所有雲端，附每檔案雲端徽章、路徑導覽與即時各雲容量儀表板
- 🔍 **全域即時搜尋** — 一次輸入即可透過本機全文索引（SQLite FTS5）搜尋所有已連結帳號
- 🧠 **智慧放置** — 上傳自動送往剩餘空間最大的雲端。可逐次覆寫，或在規則編輯器中設定規則（*"影片→Google，PDF→MyBox，超過 1GB→pCloud"*）
- 🚚 **跨雲傳輸** — 以串流經過你機器的背景工作在雲端之間複製或移動檔案。關掉分頁，傳輸仍會繼續
- 👁 **預覽與串流** — 圖片、可定位的影片、音訊與 PDF 預覽，以及附本機產生縮圖的圖庫格狀檢視
- 🔗 **分享連結、下載、重新命名/移動/複製/刪除** — 日常檔案操作，一站完成
- 🖥️ **本機優先** — 單一二進位檔，無帳號、無遙測。需要遠端存取？`PLEUMCLOUD_SERVER=1 PLEUMCLOUD_PASSWORD=… pleumcloud` 會以 Basic auth 保護（搭配 NAS/VPS 上的 HTTPS）
- 🧩 **18 個供應商** — 10 個手工打造的原生連接器，加上 8 個透過 rclone 橋接的供應商，全部藏在同一介面之後

## 支援的雲端服務

| 供應商 | 免費容量 | 連結方式 | 狀態 |
|---|---|---|---|
| Google Drive | 15 GB | OAuth 2.0 | ✅ 原生 |
| Microsoft OneDrive | 5 GB | OAuth 2.0 | ✅ 原生 |
| Dropbox | 2 GB | OAuth 2.0 | ✅ 原生 |
| Naver MyBox | 30 GB | 存取權杖 | ✅ 原生 |
| Drime | 20 GB | 存取權杖 | ✅ 原生 |
| pCloud | 10 GB | OAuth 2.0 | ✅ 原生 |
| Koofr | 10 GB | 電子郵件 + 權杖 | ✅ 原生 |
| WebDAV（Nextcloud、ownCloud、MagentaCloud 等） | — | URL + 登入 | ✅ 原生 |
| InfiniCLOUD | 20 GB | URL + 登入 | ✅ 原生 |
| MediaFire | 10 GB | 應用程式憑證 | ✅ 原生 |
| MEGA · Box · Yandex Disk · HiDrive · Jottacloud · Filen · Internxt · Proton Drive | 5–20 GB | [rclone](https://rclone.org) 橋接 | 🧪 實驗性 |
| iCloud Drive · Samsung Cloud · TeraBox · Sync.com | — | — | ❌ 無官方 API — [原因](../docs/decisions.md) |

我們評估過的所有服務完整矩陣，以及所有重大設計決策背後的理由，見 [docs/decisions.md](../docs/decisions.md)。

## 安裝

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/scripts/install.sh | bash
```

然後執行 `pleumcloud` —— 瀏覽器會開啟 `http://localhost:7777`。

**其他方式:**

```bash
# 從原始碼建置（Go 1.26+ 與 Node 20+）
git clone https://github.com/gachon-star-want/pleumcloud
cd pleumcloud && make build && ./pleumcloud

# Windows：從 Releases 下載 .zip
```

**首次 OAuth 連結？** 按照 [docs/oauth-setup.md](../docs/oauth-setup.md) 一次性取得免費應用程式金鑰（10 分鐘，每台機器一次）—— 之後每次連結只需一鍵。

## 運作原理

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

PleumCloud 是控制平面，而不是託管方。索引器透過各供應商的變更摘要增量同步，維護名稱、大小和日期的本機 SQLite/FTS5 目錄；放置引擎決定新檔案的去向；跨雲傳輸使用各供應商的可續傳通訊協定，以「來源 → 你的機器 → 目的地」的方式串流進行。

## 開發

需要 Go 1.26+ 與 Node 20+。

```bash
make dev    # go run + Vite 開發伺服器（熱重載）
make build  # 前端打包 → 內嵌的 Go 二進位檔
make test   # go test ./...
```

後端位於 `internal/`（api、provider、index、placement 等），React 應用程式位於 `web/`。

## 貢獻

歡迎 Issue 與 PR —— 包括**新連接器**、UI 改進、文件與翻譯。每個連接器位於 `internal/provider/<name>/`，在單一介面之後並附 mock 測試；從 [CONTRIBUTING.md](../CONTRIBUTING.md) 開始。

## 隱私

無遙測、無分析、無帳號。權杖保存在系統鑰匙圈中；索引保存在 `~/.pleumcloud/`。詳見[隱私政策](../web/public/privacy.html)。

## 授權條款

PleumCloud 是 **GNU AGPL-3.0** 自由軟體 —— 可自由使用、研究、修改與自架。若將其作為網路服務提供，必須以相同授權條款公開你的修改。

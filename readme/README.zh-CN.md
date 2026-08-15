<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">把散落的免费云存储，合并成一个盘。</h3>

<p align="center">
  <a href="https://github.com/gachon-star-want/pleumcloud/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/gachon-star-want/pleumcloud/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white&label=CI" alt="CI"></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square&logo=gnu&logoColor=white" alt="许可证：AGPL-3.0"></a>
</p>

<p align="center">
  🌍 [English](../README.md) · [한국어](README.ko.md) · [日本語](README.ja.md) · 简体中文 · [繁體中文](README.zh-TW.md) · [Español](README.es.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Português (Brasil)](README.pt-BR.md)
</p>

---

PleumCloud 是一款本地优先的应用，把你散落在各处的免费云存储 —— Google 15GB、Naver MyBox 30GB、OneDrive 5GB、Drime 20GB、pCloud 10GB…… 分布在六个应用里的 100GB 以上空间 —— 合并成**一个盘**。只需连接一次账号，就能像使用单个磁盘一样浏览、搜索、上传和移动所有云端的文件，每个文件都带有显示其所属云的徽标。

文件绝不会被重新托管：每个文件完整地留在原来的云上，PleumCloud 只保留本地索引和系统钥匙串中的凭据。它以单个约 18MB、内嵌 UI 的二进制文件发布。

## 功能特性

- 🔗 **一键连接** — Google Drive、OneDrive、Dropbox、pCloud 走官方 OAuth；Naver MyBox、Drime、Koofr、MediaFire 用访问令牌；其余用 WebDAV。凭据保存在系统钥匙串中
- 🗂️ **统一云盘** — 在单一视图中浏览所有云，带每文件云徽标、面包屑导航和实时各云配额面板
- 🔍 **全局即时搜索** — 一次输入即可通过本地全文索引（SQLite FTS5）搜索所有已连接账号
- 🧠 **智能放置** — 上传自动进入剩余空间最大的云。可按次覆盖，或在规则编辑器中设置规则（*"视频→Google，PDF→MyBox，超过 1GB→pCloud"*）
- 🚚 **跨云传输** — 以流经本机的后台任务在云之间复制或移动文件。关掉标签页，传输仍在继续
- 👁 **预览与流媒体** — 图片、可拖动进度的视频、音频和 PDF 预览，以及带本地生成缩略图的画廊网格视图
- 🔗 **分享链接、下载、重命名/移动/复制/删除** — 日常文件操作，一处完成
- 🖥️ **本地优先** — 单一二进制，无账号，无遥测。需要远程访问？`PLEUMCLOUD_SERVER=1 PLEUMCLOUD_PASSWORD=… pleumcloud` 会用 Basic auth 保护（配合 NAS/VPS 上的 HTTPS）
- 🧩 **18 个提供商** — 10 个手工打造的原生连接器 + 8 个通过 rclone 桥接，全部在同一接口之后

## 支持的云服务

| 提供商 | 免费额度 | 连接方式 | 状态 |
|---|---|---|---|
| Google Drive | 15 GB | OAuth 2.0 | ✅ 原生 |
| Microsoft OneDrive | 5 GB | OAuth 2.0 | ✅ 原生 |
| Dropbox | 2 GB | OAuth 2.0 | ✅ 原生 |
| Naver MyBox | 30 GB | 访问令牌 | ✅ 原生 |
| Drime | 20 GB | 访问令牌 | ✅ 原生 |
| pCloud | 10 GB | OAuth 2.0 | ✅ 原生 |
| Koofr | 10 GB | 邮箱 + 令牌 | ✅ 原生 |
| WebDAV（Nextcloud、ownCloud、MagentaCloud 等） | — | URL + 登录 | ✅ 原生 |
| InfiniCLOUD | 20 GB | URL + 登录 | ✅ 原生 |
| MediaFire | 10 GB | 应用凭据 | ✅ 原生 |
| MEGA · Box · Yandex Disk · HiDrive · Jottacloud · Filen · Internxt · Proton Drive | 5–20 GB | [rclone](https://rclone.org) 桥接 | 🧪 实验性 |
| iCloud Drive · Samsung Cloud · TeraBox · Sync.com | — | — | ❌ 无官方 API — [原因](../docs/decisions.md) |

我们评估过的所有服务的完整矩阵，以及所有重大设计决策背后的理由，见 [docs/decisions.md](../docs/decisions.md)。

## 安装

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/scripts/install.sh | bash
```

然后运行 `pleumcloud` —— 浏览器会打开 `http://localhost:7777`。

**其他方式:**

```bash
# 从源码构建（Go 1.26+ 和 Node 20+）
git clone https://github.com/gachon-star-want/pleumcloud
cd pleumcloud && make build && ./pleumcloud

# Windows：从 Releases 下载 .zip
```

**首次 OAuth 连接？** 按照 [docs/oauth-setup.md](../docs/oauth-setup.md) 一次性领取免费应用密钥（10 分钟，每台机器一次）—— 之后每次连接只需一键。

## 工作原理

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

PleumCloud 是控制平面，而不是托管方。索引器通过各提供商的变更源增量同步，维护名称、大小和日期的本地 SQLite/FTS5 目录；放置引擎决定新文件的去向；跨云传输使用各提供商的可续传协议，以"源 → 你的机器 → 目的地"的方式流式进行。

## 开发

需要 Go 1.26+ 和 Node 20+。

```bash
make dev    # go run + Vite 开发服务器（热重载）
make build  # 前端打包 → 内嵌的 Go 二进制
make test   # go test ./...
```

后端位于 `internal/`（api、provider、index、placement 等），React 应用位于 `web/`。

## 贡献

欢迎 Issue 和 PR —— 包括**新连接器**、UI 改进、文档和翻译。每个连接器位于 `internal/provider/<name>/`，在单一接口之后并带 mock 测试；从 [CONTRIBUTING.md](../CONTRIBUTING.md) 开始。

## 隐私

无遥测、无分析、无账号。令牌保存在系统钥匙串中；索引保存在 `~/.pleumcloud/`。详见[隐私政策](../web/public/privacy.html)。

## 许可证

PleumCloud 是 **GNU AGPL-3.0** 自由软件 —— 可自由使用、研究、修改和自托管。若将其作为网络服务提供，必须以相同许可证公开你的修改。

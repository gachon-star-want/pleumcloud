<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">散らばった無料クラウドストレージを、ひとつのドライブに。</h3>

<p align="center">
  <a href="https://github.com/gachon-star-want/pleumcloud/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/gachon-star-want/pleumcloud/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white&label=CI" alt="CI"></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square&logo=gnu&logoColor=white" alt="ライセンス: AGPL-3.0"></a>
</p>

<p align="center">
  🌍 [English](../README.md) · [한국어](README.ko.md) · 日本語 · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · [Español](README.es.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Português (Brasil)](README.pt-BR.md)
</p>

---

PleumCloudは、散らばった無料クラウドストレージ — Google 15GB、Naver MyBox 30GB、OneDrive 5GB、Drime 20GB、pCloud 10GB… 6つのアプリに分かれた100GB超の容量 — を**ひとつのドライブ**にまとめるローカルファースト・アプリです。アカウントを一度接続すれば、すべてのクラウドのファイルをひとつのディスクのように閲覧・検索・アップロード・移動でき、各ファイルにどのクラウドにあるかを示すバッジが付きます。

ファイルは決して再ホストされません。各ファイルはひとつのクラウドに丸ごと置かれ、PleumCloudが保持するのはローカルインデックスと、OSキーチェーン内の資格情報だけです。UIを組み込んだ単一の約18MBバイナリとして配布されます。

## 主な機能

- 🔗 **ワンクリック接続** — Google Drive・OneDrive・Dropbox・pCloudは公式OAuth、Naver MyBox・Drime・Koofr・MediaFireはアクセストークン、その他はWebDAV。資格情報はOSキーチェーンに保存
- 🗂️ **統一ドライブ** — ファイルごとのクラウドバッジ、パンくずリスト、クラウド別の容量ダッシュボード付きの単一ビューで全クラウドを閲覧
- 🔍 **全体を即座に検索** — ローカル全文インデックス（SQLite FTS5）で、接続済みの全アカウントを一度の入力で検索
- 🧠 **スマート配置** — アップロードは空き容量が最も大きいクラウドへ自動で。アップロード単位で上書き、またはルールエディターでルールを設定（*「動画→Google、PDF→MyBox、1GB超→pCloud」*）
- 🚚 **クラウド間転送** — マシンを経由してストリーミングするバックグラウンドジョブでクラウド間をコピー・移動。タブを閉じても転送は継続
- 👁 **プレビューとストリーミング** — 画像、シーク可能な動画、音声、PDFのプレビューに加え、ローカル生成サムネイル付きのギャラリーグリッドビュー
- 🔗 **共有リンク・ダウンロード・名前変更/移動/コピー/削除** — 日常的なファイル操作をひとつの場所で
- 🖥️ **ローカルファースト** — 単一バイナリ、アカウント不要、テレメトリなし。リモートアクセスが必要なら `PLEUMCLOUD_SERVER=1 PLEUMCLOUD_PASSWORD=… pleumcloud` でBasic認証付きに（NAS/VPSのHTTPSと併用）
- 🧩 **18プロバイダー** — 手作りのネイティブコネクター10種とrcloneブリッジ経由8種を、すべてひとつのインターフェースの下で

## 対応クラウド

| プロバイダー | 無料容量 | 接続方法 | 状態 |
|---|---|---|---|
| Google Drive | 15 GB | OAuth 2.0 | ✅ ネイティブ |
| Microsoft OneDrive | 5 GB | OAuth 2.0 | ✅ ネイティブ |
| Dropbox | 2 GB | OAuth 2.0 | ✅ ネイティブ |
| Naver MyBox | 30 GB | アクセストークン | ✅ ネイティブ |
| Drime | 20 GB | アクセストークン | ✅ ネイティブ |
| pCloud | 10 GB | OAuth 2.0 | ✅ ネイティブ |
| Koofr | 10 GB | メール + トークン | ✅ ネイティブ |
| WebDAV（Nextcloud、ownCloud、MagentaCloud など） | — | URL + ログイン | ✅ ネイティブ |
| InfiniCLOUD | 20 GB | URL + ログイン | ✅ ネイティブ |
| MediaFire | 10 GB | アプリ資格情報 | ✅ ネイティブ |
| MEGA · Box · Yandex Disk · HiDrive · Jottacloud · Filen · Internxt · Proton Drive | 5–20 GB | [rclone](https://rclone.org) ブリッジ | 🧪 実験的 |
| iCloud Drive · Samsung Cloud · TeraBox · Sync.com | — | — | ❌ 公式APIなし — [理由](../docs/decisions.md) |

評価対象に挙げた全サービスのマトリクスと、主要な設計判断の背景は [docs/decisions.md](../docs/decisions.md) にあります。

## インストール

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/scripts/install.sh | bash
```

その後 `pleumcloud` を実行すると、ブラウザが `http://localhost:7777` で開きます。

**その他の方法:**

```bash
# ソースからビルド（Go 1.26+、Node 20+）
git clone https://github.com/gachon-star-want/pleumcloud
cd pleumcloud && make build && ./pleumcloud

# Windows: Releasesから.zipをダウンロード
```

**初めてのOAuth接続？** [docs/oauth-setup.md](../docs/oauth-setup.md) に従って無料のアプリキーを一度だけ取得すれば（10分、マシンごとに1回）、以降の接続はすべてワンクリックです。

## 仕組み

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

PleumCloudはコントロールプレーンであり、ホストではありません。インデクサーは各プロバイダーの変更フィードで差分同期しながら、名前・サイズ・日付のローカルSQLite/FTS5カタログを維持します。配置エンジンが新規ファイルの行き先を決め、クラウド間転送は各プロバイダーの再開可能アップロードで ソース → あなたのマシン → 宛先 へとストリーミングされます。

## 開発

Go 1.26+ と Node 20+ が必要です。

```bash
make dev    # go run + Vite開発サーバー（ホットリロード）
make build  # フロントエンドバンドル → 組み込みGoバイナリ
make test   # go test ./...
```

バックエンドは `internal/`（api、provider、index、placement など）、Reactアプリは `web/` にあります。

## コントリビュート

イシューとPRを歓迎します — **新しいコネクター**、UIの改善、ドキュメントや翻訳も。各コネクターは `internal/provider/<name>/` に、単一インターフェースとモックテスト付きで置かれています。[CONTRIBUTING.md](../CONTRIBUTING.md) から始めてください。

## プライバシー

テレメトリもアナリティクスもアカウントもありません。トークンはOSキーチェーンに、インデックスは `~/.pleumcloud/` に保存されます。詳細: [プライバシーポリシー](../web/public/privacy.html)。

## ライセンス

PleumCloudは**GNU AGPL-3.0**のフリーソフトウェアです — 自由に使用・改変・セルフホストでき、ネットワークサービスとして提供する場合は改変部分を同じライセンスで公開する必要があります。

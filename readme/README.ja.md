<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">無料クラウドストレージを、ひとつのドライブに。</h3>

> 🌍 [English](../README.md) · 日本語

Google 15GB、Naver MyBox 30GB、OneDrive 5GB、pCloud 10GB…あなたはすでに**100GB超の無料クラウド**を持っていますが、6つのアプリに散在し、ログインもアップロードも検索もバラバラです。

**PleumCloudはそれをひとつのドライブにまとめます。** アカウントを一度接続すれば、すべてのクラウドのファイルをひとつの画面で閲覧・検索・アップロード・移動でき、各ファイルにどのクラウドにあるかのバッジが付きます。

> 2026年8月、Naverが13年ぶりに[MyBox Open API](https://developers.mybox.naver.com)を開放したその週に生まれました。

## ✨ 主な機能

- 🔗 **ワンクリック接続** — Google・OneDrive・Dropbox・pCloud は OAuth、MyBox・Drime・Koofr はアクセストークン、その他は WebDAV。認証情報は OS キーチェーンに保存
- 🗂️ **統合ドライブ** — クラウドバッジ付きの統合ビュー + クラウド別容量ダッシュボード
- 🔍 **横断検索** — 接続した全アカウントをローカル全文索引から一括検索
- 🧠 **スマート配置** — アップロードは空き容量が最大のクラウドへ自動配置。ルール設定も可能
- 🚚 **クラウド間転送** — バックグラウンドジョブでストリーミング。タブを閉じても継続
- 🖥️ **ローカルファースト** — UI 組み込みの単一バイナリ(約18MB)。サーバー不要
- 🧩 **17プロバイダー** — ネイティブ9種 + rclone ブリッジ8種

## インストール (macOS / Linux)

```bash
curl -fsSL https://pleumcloud.dev/install.sh | bash
```

`pleumcloud` を実行すると、ブラウザが `http://localhost:7777` で開きます。

## ライセンス

**GNU AGPL-3.0**

---

<p align="center">☁️ <i>[Discover_it](https://github.com/gachon-star-want) とコントリビューターによって作られました。</i></p>

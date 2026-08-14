<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">把你所有的免费云存储，变成一块硬盘。</h3>

> 🌍 [English](../README.md) · 简体中文

Google 15GB、Naver MyBox 30GB、OneDrive 5GB、pCloud 10GB……你已经拥有**超过 100GB 的免费云存储**，却散落在六个应用里，各自的登录、上传和搜索互不相通。

**PleumCloud 把它们合并成一块硬盘。** 连接一次账户，就能在同一个界面里浏览、搜索、上传和移动所有云端的文件——每个文件都带着标签，显示它住在哪个云上。

> 诞生于 2026 年 8 月——Naver 时隔 13 年开放 [MyBox Open API](https://developers.mybox.naver.com) 的那一周。

## ✨ 主要特性

- 🔗 **一键连接** — Google、OneDrive、Dropbox、pCloud 走 OAuth;MyBox、Drime、Koofr 走访问令牌；其余走 WebDAV。凭据保存在系统钥匙串
- 🗂️ **统一硬盘** — 带云端徽章的统一视图 + 实时容量仪表盘
- 🔍 **全局搜索** — 一次输入，搜索所有已连接账户(本地全文索引)
- 🧠 **智能放置** — 上传自动放入剩余空间最大的云，也可设置规则(“视频→Google,PDF→MyBox”)
- 🚚 **跨云传输** — 后台任务流式传输，关掉标签页也在继续
- 🖥️ **本地优先** — 单个约 18MB 的二进制，内嵌界面，无需服务器
- 🧩 **17 个云服务** — 9 个原生连接器 + 8 个 rclone 桥接

## 安装 (macOS / Linux)

```bash
curl -fsSL https://pleumcloud.dev/install.sh | bash
```

运行 `pleumcloud`,浏览器会自动打开 `http://localhost:7777`。

## 许可证

**GNU AGPL-3.0**

---

<p align="center">☁️ <i>由 [Discover_it](https://github.com/gachon-star-want) 和贡献者构建——100GB 的免费存储空间值得一个统一的家。</i></p>

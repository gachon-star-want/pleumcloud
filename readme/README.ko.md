<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">흩어진 무료 클라우드, 하나의 드라이브로.</h3>

<p align="center">
  <a href="https://github.com/gachon-star-want/pleumcloud/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/gachon-star-want/pleumcloud/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white&label=CI" alt="CI"></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square&logo=gnu&logoColor=white" alt="라이선스: AGPL-3.0"></a>
</p>

<p align="center">
  🌍 [English](../README.md) · 한국어 · [日本語](README.ja.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · [Español](README.es.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Português (Brasil)](README.pt-BR.md)
</p>

---

PleumCloud는 흩어져 있는 무료 클라우드 저장소 — 구글 15GB, 네이버 마이박스 30GB, OneDrive 5GB, 드라임 20GB, pCloud 10GB… 여섯 개 앱에 나뉜 100GB 넘는 공간 — 을 **하나의 드라이브로** 묶는 로컬-퍼스트 앱입니다. 계정을 한 번만 연결하면 모든 클라우드의 파일을 하나의 디스크처럼 탐색·검색·업로드·이동할 수 있고, 파일마다 어느 클라우드에 있는지 배지로 표시됩니다.

파일은 절대 재호스팅되지 않습니다. 각 파일은 한 클라우드에 통째로 남고, PleumCloud는 로컬 인덱스와 OS 키체인의 자격증명만 관리합니다. UI를 내장한 단일 ~18MB 바이너리로 배포됩니다.

## 주요 기능

- 🔗 **원클릭 연결** — 구글 드라이브·OneDrive·Dropbox·pCloud는 공식 OAuth, 마이박스·드라임·Koofr·MediaFire는 액세스 토큰, 그 외는 WebDAV. 자격증명은 OS 키체인에 보관
- 🗂️ **하나의 통합 드라이브** — 파일별 클라우드 배지, 경로 이동(브레드크럼), 실시간 클라우드별 용량 대시보드가 붙은 단일 화면에서 모든 클라우드 탐색
- 🔍 **통합 즉시 검색** — 한 번의 입력으로 로컬 전문 색인(SQLite FTS5)에서 모든 연결 계정을 검색
- 🧠 **스마트 배치** — 업로드는 여유 공간이 가장 큰 클라우드로 자동. 업로드 단위로 직접 지정하거나, 규칙 에디터에서 규칙 설정 (*"동영상→구글, PDF→마이박스, 1GB 이상→pCloud"*)
- 🚚 **클라우드 간 전송** — 내 장비를 경유해 스트리밍하는 백그라운드 잡으로 클라우드 간 복사·이동. 브라우저를 닫아도 전송은 계속
- 👁 **미리보기·스트리밍** — 이미지, 시크 가능한 동영상, 오디오, PDF 미리보기 + 로컬 생성 썸네일이 붙은 갤러리 그리드 뷰
- 🔗 **공유 링크·다운로드·이름 변경/이동/복사/삭제** — 일상적인 파일 작업을 한곳에서
- 🖥️ **로컬 퍼스트** — 단일 바이너리, 계정 없음, 텔레메트리 없음. 원격 접속이 필요하면 `PLEUMCLOUD_SERVER=1 PLEUMCLOUD_PASSWORD=… pleumcloud`로 Basic 인증 보호 (NAS/VPS의 HTTPS와 조합)
- 🧩 **18개 프로바이더** — 직접 만든 네이티브 커넥터 10종 + rclone 브리지 8종, 모두 하나의 인터페이스 뒤에서

## 지원 클라우드

| 프로바이더 | 무료 용량 | 연결 방식 | 상태 |
|---|---|---|---|
| Google Drive | 15 GB | OAuth 2.0 | ✅ 네이티브 |
| Microsoft OneDrive | 5 GB | OAuth 2.0 | ✅ 네이티브 |
| Dropbox | 2 GB | OAuth 2.0 | ✅ 네이티브 |
| 네이버 마이박스 | 30 GB | 액세스 토큰 | ✅ 네이티브 |
| Drime | 20 GB | 액세스 토큰 | ✅ 네이티브 |
| pCloud | 10 GB | OAuth 2.0 | ✅ 네이티브 |
| Koofr | 10 GB | 이메일 + 토큰 | ✅ 네이티브 |
| WebDAV (Nextcloud, ownCloud, MagentaCloud 등) | — | URL + 로그인 | ✅ 네이티브 |
| InfiniCLOUD | 20 GB | URL + 로그인 | ✅ 네이티브 |
| MediaFire | 10 GB | 앱 자격증명 | ✅ 네이티브 |
| MEGA · Box · Yandex Disk · HiDrive · Jottacloud · Filen · Internxt · Proton Drive | 5–20 GB | [rclone](https://rclone.org) 브리지 | 🧪 실험적 |
| iCloud Drive · 삼성 클라우드 · TeraBox · Sync.com | — | — | ❌ 공식 API 없음 — [이유](../docs/decisions.md) |

평가했던 모든 서비스의 전체 매트릭스와 주요 설계 결정의 배경 근거는 [docs/decisions.md](../docs/decisions.md)에 있습니다.

## 설치

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/scripts/install.sh | bash
```

그 다음 `pleumcloud`를 실행하면 브라우저가 `http://localhost:7777`에 열립니다.

**다른 방법:**

```bash
# 소스에서 빌드 (Go 1.26+, Node 20+)
git clone https://github.com/gachon-star-want/pleumcloud
cd pleumcloud && make build && ./pleumcloud

# Windows: Releases에서 .zip 다운로드
```

**첫 OAuth 연결?** [docs/oauth-setup.md](../docs/oauth-setup.md)의 안내대로 무료 앱 키를 한 번만 만들면(10분, 장비당 1회) 이후 연결은 전부 원클릭입니다.

## 작동 방식

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

PleumCloud는 호스트가 아니라 컨트롤 플레인입니다. 인덱서는 각 프로바이더의 변경 피드로 증분 동기화하며 이름·크기·날짜의 로컬 SQLite/FTS5 카탈로그를 유지하고, 배치 엔진은 새 파일의 행선지를 정하며, 클라우드 간 전송은 각 프로바이더의 재개 가능 업로드 프로토콜로 원본 → 내 장비 → 대상을 스트리밍합니다.

## 개발

Go 1.26+와 Node 20+가 필요합니다.

```bash
make dev    # go run + Vite 개발 서버 (핫 리로드)
make build  # 프론트엔드 번들 → 임베드 Go 바이너리
make test   # go test ./...
```

백엔드는 `internal/`(api, provider, index, placement 등), React 앱은 `web/`에 있습니다.

## 기여

이슈와 PR 모두 환영합니다 — **새 커넥터**, UI 개선, 문서와 번역도요. 각 커넥터는 `internal/provider/<name>/`에 단일 인터페이스와 목업 테스트를 갖춘 형태로 있습니다. [CONTRIBUTING.md](../CONTRIBUTING.md)부터 시작하세요.

## 프라이버시

텔레메트리·분석·계정 없음. 토큰은 OS 키체인에, 인덱스는 `~/.pleumcloud/`에 저장됩니다. 자세히: [개인정보 처리방침](../web/public/privacy.html).

## 라이선스

PleumCloud는 **GNU AGPL-3.0** 자유 소프트웨어입니다 — 자유롭게 사용·수정·셀프호스팅할 수 있으며, 네트워크 서비스로 제공하는 경우 수정분을 같은 라이선스로 공개해야 합니다.

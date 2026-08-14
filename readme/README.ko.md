<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">흩어진 무료 클라우드, 하나의 드라이브로.</h3>

> 🌍 [English](../README.md) · 한국어

구글 15GB, 마이박스 30GB, 원드라이브 5GB, 드라임 20GB, pCloud 10GB… 이미 **100GB가 넘는 무료 클라우드**를 갖고 계시지만, 여섯 개 앱에 흩어져 있고 각자 로그인·업로드·검색이 따로 놀죠.

**PleumCloud는 이걸 하나의 드라이브로 묶습니다.** 계정을 한 번만 연결하면, 모든 클라우드의 파일을 하나의 화면에서 탐색·검색·업로드·이동할 수 있고, 파일마다 어느 클라우드에 있는지 배지로 표시됩니다.

> 2026년 8월, 네이버가 13년 만에 [마이박스 Open API](https://developers.mybox.naver.com)를 연 그주에 태어났습니다 — 마지막까지 닫혀 있던 한국의 큰 클라우드가 문을 열었고, 흩어진 무료 저장소가 하나로 뭉칠 이유가 생겼습니다.

## ✨ 주요 기능

- 🔗 **원클릭 연결** — 구글·OneDrive·Dropbox·pCloud는 OAuth, 마이박스·드라임·Koofr는 액세스 토큰, 그 외는 WebDAV. 자격증명은 OS 키체인에 보관
- 🗂️ **하나의 통합 드라이브** — 클라우드 배지가 붙은 통합 뷰 + 실시간 클라우드별 용량 대시보드
- 🔍 **통합 검색** — 한 번의 입력으로 모든 연결 계정을 로컬 전문색인에서 검색
- 🧠 **스마트 배치** — 업로드는 여유 공간이 가장 큰 클라우드로 자동. 규칙 설정 가능 ("동영상→구글, PDF→마이박스, 1GB 이상→pCloud")
- 🚚 **클라우드 간 전송** — 백그라운드 잡으로 스트리밍. 브라우저를 닫아도 계속
- 🖥️ **로컬 퍼스트** — UI 내장 단일 바이너리(~18MB). 계정·텔레메트리·서버 불필요 (셀프호스팅 서버 모드는 로드맵에)
- 🧩 **17개 프로바이더** — 네이티브 9종 + rclone 브리지 8종

## 설치 (macOS / Linux)

```bash
curl -fsSL https://pleumcloud.dev/install.sh | bash
```

실행하면 `pleumcloud` — 브라우저가 `http://localhost:7777`에 자동으로 열립니다.

첫 OAuth 연결은 [docs/oauth-setup.md](../docs/oauth-setup.md)의 안내대로 앱 키를 한 번만 붙여넣으면 이후엔 원클릭입니다.

## 지원 클라우드

Google Drive · OneDrive · Dropbox · 네이버 마이박스 · Drime · pCloud · Koofr · WebDAV(Nextcloud 등) · InfiniCLOUD — **네이티브 지원**
MEGA · Box · Yandex · HiDrive · Jottacloud · Filen · Internxt · Proton Drive — **rclone 브리지로 실험 지원**

iCloud·삼성 클라우드·TeraBox는 공식 API가 없어 제외했습니다 ([이유](../docs/provider-decisions.md)).

## 라이선스

**GNU AGPL-3.0** — 자유롭게 사용·수정·셀프호스팅할 수 있으며, 네트워크 서비스로 제공 시 수정분을 같은 라이선스로 공개해야 합니다.

---

<p align="center">☁️ <i>[Discover_it](https://github.com/gachon-star-want)와 기여자들이 만들었습니다 — 100GB의 무료 저장소는 하나의 집을 가질 자격이 있으니까요.</i></p>

import { createContext, useContext, useState, type ReactNode } from "react";

type Lang = "en" | "ko";

const dict: Record<string, [string, string]> = {
  // en, ko
  allDrives: ["All Drives", "전체 드라이브"],
  recent: ["Recent", "최근"],
  starred: ["Starred", "즐겨찾기"],
  trash: ["Trash", "휴지통"],
  settings: ["Settings", "설정"],
  uploadRules: ["upload rules", "업로드 규칙"],
  clouds: ["Clouds", "클라우드"],
  reconnectNeeded: ["Reconnect needed — token rejected", "재연결 필요 — 토큰이 거부됐습니다"],
  connectCloud: ["Connect a cloud", "클라우드 연결"],
  noCloudsYet: ["No clouds connected yet.", "연결된 클라우드가 없습니다."],
  usageAfter: ["Usage appears once you connect clouds.", "클라우드를 연결하면 사용량이 표시됩니다."],
  used: ["used", "사용중"],
  searchPlaceholder: ["Search across all your clouds…", "모든 클라우드에서 검색…"],
  loading: ["Loading…", "불러오는 중…"],
  nothingHere: ["Nothing here yet", "아직 아무것도 없습니다"],
  connectPrompt: ["Connect a cloud and it will appear here after syncing.", "클라우드를 연결하면 동기화 후 여기에 표시됩니다."],
  dropOrFolder: ["Drop a file or create a folder.", "파일을 올리거나 폴더를 만드세요."],
  newFolder: ["+ Folder", "+ 폴더"],
  upload: ["⬆ Upload", "⬆ 업로드"],
  uploading: ["Uploading…", "업로드 중…"],
  gallery: ["🖼 Gallery", "🖼 갤러리"],
  list: ["▤ List", "▤ 목록"],
  fullSupport: ["Full support", "완전 지원"],
  oneClick: ["One-click connect with official APIs.", "공식 API로 원클릭 연결."],
  experimental: ["Experimental", "실험적 지원"],
  experimentalDesc: ["Served through the rclone bridge — connect with credentials from each service.", "rclone 브리지로 제공 — 각 서비스의 자격증명으로 연결합니다."],
  connected: ["Connected", "연결됨"],
  disconnect: ["Disconnect", "연결 해제"],
  connect: ["Connect", "연결"],
  soon: ["Soon", "준비중"],
  results: ["results for", "개 결과:"],
  searching: ["Searching…", "검색 중…"],
  rulesTitle: ["Upload rules", "업로드 규칙"],
  rulesDesc: ["New uploads match rules in priority order (lower number first); the first match decides the cloud. No match → most free space.", "새 업로드는 우선순위 순서(숫자가 낮을수록 먼저)로 규칙과 대조되고, 첫 일치가 클라우드를 정합니다. 일치 없음 → 여유 공간 최대."],
  addRule: ["Add rule", "규칙 추가"],
  signIn: ["Sign in", "로그인"],
  signUp: ["Create account", "계정 만들기"],
  email: ["Email", "이메일"],
  password: ["Password", "비밀번호"],
  passwordHint: ["8+ characters", "8자 이상"],
  welcome: ["Welcome to PleumCloud", "PleumCloud에 오신 것을 환영합니다"],
  welcomeDesc: ["All your free cloud storage, one drive. Sign in to see your clouds.", "흩어진 무료 클라우드, 하나의 드라이브. 로그인해서 내 클라우드를 확인하세요."],
  logout: ["Log out", "로그아웃"],
  transferTo: ["Copy to another cloud", "다른 클라우드로 복사"],
  transferDesc: ["Connect another cloud to transfer between them.", "클라우드 간 전송을 위해 다른 클라우드를 연결하세요."],
  runsInBackground: ["runs in background", "백그라운드에서 실행"],
  copy: ["Copy", "복사"],
  // Lightbox preview
  fit: ["Fit", "맞춤"],
  actualSize: ["Actual size", "실제 크기"],
  download: ["Download", "다운로드"],
  openOriginal: ["Open original in a new tab", "새 탭에서 원본 열기"],
  close: ["Close", "닫기"],
  noPreview: ["No inline preview for this type.", "이 형식은 인라인 미리보기를 지원하지 않습니다."],
  downloadInstead: ["Download instead", "대신 다운로드"],
  preparingPreview: ["Preparing preview…", "미리보기 준비 중…"],
  // Connect UI
  cloudConnected: ["connected", "연결됨"],
  openTokenPage: ["Create a token at {name} ↗", "{name} 사이트에서 토큰 만들기 ↗"],
  tokenStepsDrime: ["Log in, then open Settings → Developer and create an API token.", "로그인 후 설정 → Developer(개발자)에서 API 토큰을 만드세요."],
  tokenStepsMybox: ["Create a personal access token (mbx_pat_…) in the MyBox developer portal.", "MyBox 개발자 포털에서 개인 액세스 토큰(mbx_pat_…)을 발급받으세요."],
  tokenStepsKoofr: ["Open Settings → API tokens and generate a new token.", "설정 → API 토큰에서 새 토큰을 생성하세요."],
  invalidGdriveId: ["Google Client IDs look like 1234-abc.apps.googleusercontent.com — copy it exactly from Google Cloud Console → APIs & Services → Credentials.", "구글 Client ID는 1234-abc.apps.googleusercontent.com 형태입니다. Google Cloud Console → API 및 서비스 → 사용자 인증 정보에서 정확히 복사해서 붙여넣어 주세요."],
  byoGuide: ["Step-by-step guide ↗", "단계별 가이드 ↗"],
};

const LangCtx = createContext<{ lang: Lang; setLang: (l: Lang) => void }>({ lang: "en", setLang: () => {} });

export function LangProvider({ children }: { children: ReactNode }) {
  const [lang, setLang] = useState<Lang>(() => (localStorage.getItem("pc-lang") as Lang) || (navigator.language.startsWith("ko") ? "ko" : "en"));
  const set = (l: Lang) => {
    localStorage.setItem("pc-lang", l);
    setLang(l);
  };
  return <LangCtx.Provider value={{ lang, setLang: set }}>{children}</LangCtx.Provider>;
}

export function useLang() {
  return useContext(LangCtx);
}

export function useT() {
  const { lang } = useLang();
  return (key: string): string => {
    const entry = dict[key];
    if (!entry) return key;
    return lang === "ko" ? entry[1] : entry[0];
  };
}

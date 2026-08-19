import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type FileRow, type ProviderMeta } from "./api";
import Sidebar from "./components/Sidebar";
import TopBar from "./components/TopBar";
import EmptyState from "./components/EmptyState";
import ConnectPanel from "./components/ConnectPanel";
import FileBrowser from "./components/FileBrowser";
import SearchResults from "./components/SearchResults";
import UpdateBanner from "./components/UpdateBanner";
import { useT } from "./i18n";

export interface Crumb {
  remoteId: string;
  name: string;
  accountId: string;
}

interface NavState {
  view: "drive" | "connect";
  crumbs: Crumb[];
  preview: FileRow | null;
}

// Folder trails serialize into a single dot-free URL segment (/d/<base64url>)
// so the server's SPA fallback (which 404s extension-looking paths) always
// serves the shell, and a reload restores the exact breadcrumb path.
function encodeTrail(crumbs: Crumb[]): string {
  const bytes = new TextEncoder().encode(JSON.stringify(crumbs));
  let bin = "";
  bytes.forEach((b) => (bin += String.fromCharCode(b)));
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function decodeTrail(seg: string): Crumb[] | null {
  try {
    const bin = atob(seg.replace(/-/g, "+").replace(/_/g, "/"));
    const json = new TextDecoder().decode(Uint8Array.from(bin, (c) => c.charCodeAt(0)));
    const arr = JSON.parse(json);
    if (!Array.isArray(arr)) return null;
    const ok = arr.every(
      (c) => c && typeof c.remoteId === "string" && typeof c.name === "string" && typeof c.accountId === "string",
    );
    return ok ? (arr as Crumb[]) : null;
  } catch {
    return null;
  }
}

function urlFor(s: NavState): string {
  if (s.view === "connect") return "/connect";
  if (s.crumbs.length === 0) return "/";
  return "/d/" + encodeTrail(s.crumbs);
}

function initialNav(): NavState {
  const path = window.location.pathname;
  if (path === "/connect") return { view: "connect", crumbs: [], preview: null };
  if (path.startsWith("/d/")) {
    return { view: "drive", crumbs: decodeTrail(path.slice(3)) ?? [], preview: null };
  }
  return { view: "drive", crumbs: [], preview: null };
}

type HistState = NavState & { __pc: boolean };

const snapshot = (s: NavState): HistState => ({ ...s, __pc: true });

export default function App() {
  const t = useT();
  const [nav, setNav] = useState<NavState>(initialNav);
  const [query, setQuery] = useState("");
  const [toast, setToast] = useState<string | null>(null);
  const qc = useQueryClient();
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const connected = accounts.data?.accounts ?? [];

  // Seed the current entry with a state snapshot; turn the OAuth success
  // redirect (/?connected=<provider>) into a toast and clean the URL.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const joined = params.get("connected");
    if (joined) {
      qc.invalidateQueries({ queryKey: ["accounts"] });
      qc.invalidateQueries({ queryKey: ["credentials"] });
      const provs = qc.getQueryData<{ providers: ProviderMeta[] }>(["providers"]);
      const name = provs?.providers.find((p) => p.id === joined)?.name ?? joined;
      setToast(`${name} ${t("cloudConnected")} ✓`);
      params.delete("connected");
      const rest = params.toString();
      window.history.replaceState(
        snapshot(initialNav()),
        "",
        window.location.pathname + (rest ? `?${rest}` : ""),
      );
      return;
    }
    window.history.replaceState(snapshot(nav), "", window.location.pathname + window.location.search);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!toast) return;
    const id = setTimeout(() => setToast(null), 5000);
    return () => clearTimeout(id);
  }, [toast]);

  // Browser back/forward restores the snapshot pushed with each entry.
  useEffect(() => {
    const onPop = (e: PopStateEvent) => {
      const s = e.state as HistState | null;
      if (s && s.__pc) setNav({ view: s.view, crumbs: s.crumbs ?? [], preview: s.preview ?? null });
      else setNav(initialNav());
    };
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  const navigate = (next: NavState) => {
    window.history.pushState(snapshot(next), "", urlFor(next));
    setNav(next);
  };

  const openCrumbs = (crumbs: Crumb[]) => navigate({ view: "drive", crumbs, preview: null });
  const openPreview = (file: FileRow) => navigate({ view: "drive", crumbs: nav.crumbs, preview: file });
  const closePreview = () => {
    // The preview got its own history entry when opened; popping it lets
    // Back from the pre-preview state also work.
    const st = window.history.state as HistState | null;
    if (st?.preview) window.history.back();
    else setNav((n) => ({ ...n, preview: null }));
  };

  const searching = query.trim().length > 0;

  return (
    <div className="flex h-full">
      <Sidebar connectedCount={connected.length} onConnect={() => navigate({ view: "connect", crumbs: [], preview: null })} />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar
          view={nav.view}
          query={query}
          onQuery={setQuery}
          onHome={() => {
            navigate({ view: "drive", crumbs: [], preview: null });
            setQuery("");
          }}
          onConnect={() => navigate({ view: "connect", crumbs: [], preview: null })}
        />
        <main className="flex-1 overflow-y-auto p-4 sm:p-6">
          <UpdateBanner />
          {toast && (
            <button
              onClick={() => setToast(null)}
              className="mb-3 w-full rounded-xl bg-emerald-600 px-4 py-2.5 text-left text-sm font-medium text-white"
            >
              {toast} <span className="float-right opacity-60">✕</span>
            </button>
          )}
          {nav.view === "connect" ? (
            <ConnectPanel />
          ) : searching ? (
            <SearchResults query={query.trim()} />
          ) : connected.length > 0 ? (
            <FileBrowser
              onConnect={() => navigate({ view: "connect", crumbs: [], preview: null })}
              crumbs={nav.crumbs}
              onCrumbs={openCrumbs}
              preview={nav.preview}
              onPreview={openPreview}
              onClosePreview={closePreview}
            />
          ) : (
            <EmptyState hasAccounts={false} onConnect={() => navigate({ view: "connect", crumbs: [], preview: null })} />
          )}
        </main>
      </div>
    </div>
  );
}

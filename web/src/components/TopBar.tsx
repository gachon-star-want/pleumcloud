import type { ReactNode } from "react";
import { useT } from "../i18n";

interface TopBarProps {
  view: "drive" | "connect";
  query: string;
  onQuery: (q: string) => void;
  onHome: () => void;
  onConnect: () => void;
  children?: ReactNode;
}

export default function TopBar({ view, query, onQuery, onHome, onConnect, children }: TopBarProps) {
  const t = useT();
  return (
    <header className="flex h-16 shrink-0 items-center gap-3 border-b border-slate-200 bg-white px-4 sm:px-6">
      {view !== "drive" && (
        <button
          onClick={onHome}
          className="shrink-0 rounded-lg px-2 py-1.5 text-sm font-medium text-slate-600 hover:bg-slate-100 sm:px-3"
        >
          ←
        </button>
      )}
      <span className="flex shrink-0 items-center gap-1.5 md:hidden">
        <span className="text-lg leading-none">☁️</span>
        <span className="font-bold tracking-tight">PleumCloud</span>
      </span>
      <h1 className="hidden shrink-0 text-lg font-semibold md:block">
        {view === "drive" ? "All Drives" : "Connect a cloud"}
      </h1>
      <div className="mx-auto w-full max-w-xl min-w-0">
        <input
          type="search"
          value={query}
          onChange={(e) => onQuery(e.target.value)}
          placeholder={t("searchPlaceholder")}
          aria-label="Search files"
          className="w-full rounded-full border border-slate-200 bg-slate-50 px-4 py-2 text-sm text-slate-700 placeholder:text-slate-400 focus:border-blue-400 focus:bg-white focus:outline-none"
        />
      </div>
      {children}
      <button
        onClick={onConnect}
        className="shrink-0 rounded-full bg-blue-600 px-3 py-1.5 text-sm font-semibold text-white hover:bg-blue-700 md:hidden"
      >
        +
      </button>
    </header>
  );
}

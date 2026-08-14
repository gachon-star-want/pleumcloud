interface TopBarProps {
  view: "drive" | "connect";
  onHome: () => void;
  onConnect: () => void;
}

export default function TopBar({ view, onHome, onConnect }: TopBarProps) {
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
          placeholder="Search across all your clouds…"
          disabled
          aria-disabled="true"
          title="Search arrives with the unified index (M4)"
          className="w-full cursor-not-allowed rounded-full border border-slate-200 bg-slate-50 px-4 py-2 text-sm text-slate-400 placeholder:text-slate-400 focus:outline-none"
        />
      </div>
      <button
        onClick={onConnect}
        className="shrink-0 rounded-full bg-blue-600 px-3 py-1.5 text-sm font-semibold text-white hover:bg-blue-700 md:hidden"
      >
        +
      </button>
      <span className="hidden shrink-0 rounded-full bg-amber-100 px-2.5 py-1 text-xs font-semibold text-amber-700 sm:inline-flex">
        M1 preview
      </span>
    </header>
  );
}

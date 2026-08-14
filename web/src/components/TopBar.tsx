interface TopBarProps {
  view: "drive" | "connect";
  onHome: () => void;
}

export default function TopBar({ view, onHome }: TopBarProps) {
  return (
    <header className="flex h-16 shrink-0 items-center gap-4 border-b border-slate-200 bg-white px-6">
      {view !== "drive" && (
        <button
          onClick={onHome}
          className="rounded-lg px-3 py-1.5 text-sm font-medium text-slate-600 hover:bg-slate-100"
        >
          ← Back
        </button>
      )}
      <h1 className="text-lg font-semibold">
        {view === "drive" ? "All Drives" : "Connect a cloud"}
      </h1>
      <div className="mx-auto w-full max-w-xl">
        <input
          type="search"
          placeholder="Search across all your clouds…"
          disabled
          className="w-full rounded-full border border-slate-200 bg-slate-50 px-4 py-2 text-sm text-slate-400 placeholder:text-slate-400 cursor-not-allowed focus:outline-none"
        />
      </div>
      <span className="rounded-full bg-amber-100 px-2.5 py-1 text-xs font-semibold text-amber-700">
        M1 preview
      </span>
    </header>
  );
}

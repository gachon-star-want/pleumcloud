import { useQuery } from "@tanstack/react-query";
import { filesApi, fmtBytes, providerDot } from "../api";
import { useT } from "../i18n";

export default function SearchResults({ query }: { query: string }) {
  const t = useT();
  const res = useQuery({
    queryKey: ["search", query],
    queryFn: () => filesApi.search(query),
  });
  const results = res.data?.results ?? [];

  return (
    <div className="mx-auto max-w-3xl">
      <p className="mb-3 text-sm text-slate-500">
        {res.isLoading ? t("searching") : `${results.length} ${t("results")} “${query}”`}
      </p>
      <div className="space-y-1.5">
        {results.map((f) => (
          <div key={f.id} className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-3">
            <span className="text-lg leading-none">{f.isDir ? "🗂️" : "📦"}</span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold text-slate-700">{f.name}</p>
              <p className="text-xs text-slate-400">{fmtBytes(f.size)}</p>
            </div>
            <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600">
              <span className="size-2 rounded-full" style={{ background: providerDot(f.providerId) }} />
              {f.accountLabel}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

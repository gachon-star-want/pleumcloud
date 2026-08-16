import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, filesApi, fmtBytes, providerDot, type ProviderMeta } from "../api";
import ProviderLogo from "./ProviderLogo";
import RulesDialog from "./RulesDialog";
import { useT } from "../i18n";

interface SidebarProps {
  connectedCount: number;
  onConnect: () => void;
}

export default function Sidebar({ connectedCount, onConnect }: SidebarProps) {
  const t = useT();
  const [showRules, setShowRules] = useState(false);
  const providers = useQuery({
    queryKey: ["providers"],
    queryFn: api.providers,
  });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const usage = useQuery({ queryKey: ["usage"], queryFn: filesApi.usage, refetchInterval: 60_000 });
  const qc = useQueryClient();
  const meta = providers.data?.providers ?? [];
  const connected = accounts.data?.accounts ?? [];
  const errorsByAccount = new Map(
    (usage.data?.usage ?? []).filter((e) => e.error).map((e) => [e.accountId, e.error as string]),
  );

  return (
    <aside className="hidden w-72 shrink-0 flex-col border-r border-slate-200 bg-white md:flex">
      <div className="flex items-center gap-2 px-5 pb-2 pt-5">
        <span className="text-2xl leading-none">☁️</span>
        <span className="text-lg font-bold tracking-tight">PleumCloud</span>
      </div>

      <nav className="px-3 py-2">
        <NavButton icon="🗂️" label={t("allDrives")} active />
        <NavButton icon="🕘" label={t("recent")} disabled />
        <NavButton icon="⭐" label={t("starred")} disabled />
        <NavButton icon="🗑️" label={t("trash")} disabled />
      </nav>

      <div className="flex items-center justify-between px-5 pt-2">
        <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">{t("settings")}</span>
        <button
          onClick={() => setShowRules(true)}
          className="text-xs font-semibold text-blue-600 hover:underline"
        >
          {t("uploadRules")}
        </button>
      </div>

      <div className="mt-2 px-5 text-xs font-semibold uppercase tracking-wider text-slate-400">
        {t("clouds")} ({connectedCount})
      </div>
      <div className="flex-1 overflow-y-auto px-3 py-2">
        {connected.map((a) => {
          const err = errorsByAccount.get(a.id);
          return (
            <div
              key={a.id}
              title={err ? `${t("reconnectNeeded")}\n${err}` : undefined}
              className="group flex items-center gap-2.5 rounded-lg px-2 py-1.5 hover:bg-slate-100"
            >
              <ProviderLogo id={a.providerId} className="size-6" />
              <span className={`min-w-0 flex-1 truncate text-sm ${err ? "text-amber-700" : "text-slate-600"}`}>
                {err ? "⚠ " : ""}
                {a.label}
              </span>
              <button
                aria-label={`Disconnect ${a.label}`}
                onClick={async () => {
                  await api.disconnect(a.id);
                  qc.invalidateQueries({ queryKey: ["accounts"] });
                }}
                className="hidden rounded px-1.5 text-xs text-slate-400 hover:text-red-600 group-hover:block"
              >
                ✕
              </button>
            </div>
          );
        })}
        <button
          onClick={onConnect}
          className="mt-1 flex w-full items-center gap-2.5 rounded-lg px-2 py-2 text-sm font-medium text-blue-600 hover:bg-blue-50"
        >
          <span className="grid size-7 place-items-center rounded-full bg-blue-100 text-base leading-none">
            +
          </span>
          {t("connectCloud")}
        </button>
      </div>

      <QuotaRibbon providers={meta} />
      {showRules && <RulesDialog onClose={() => setShowRules(false)} />}
    </aside>
  );
}

function NavButton({
  icon,
  label,
  active,
  disabled,
}: {
  icon: string;
  label: string;
  active?: boolean;
  disabled?: boolean;
}) {
  return (
    <button
      disabled={disabled}
      className={`flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm ${
        active
          ? "bg-blue-100/70 font-semibold text-blue-800"
          : disabled
            ? "cursor-not-allowed text-slate-300"
            : "text-slate-600 hover:bg-slate-100"
      }`}
    >
      <span>{icon}</span>
      {label}
    </button>
  );
}

function QuotaRibbon({ providers }: { providers: ProviderMeta[] }) {
  const t = useT();
  const usage = useQuery({ queryKey: ["usage"], queryFn: filesApi.usage, refetchInterval: 60_000 });
  const entries = usage.data?.usage ?? [];
  if (entries.length === 0) {
    return (
      <div className="border-t border-slate-200 p-4">
        <p className="mb-1.5 text-xs text-slate-500">Usage appears once you connect clouds.</p>
        <div className="flex gap-1">
          {providers.slice(0, 8).map((p) => (
            <span
              key={p.id}
              className="h-1.5 flex-1 rounded-full"
              style={{ background: providerDot(p.id), opacity: 0.35 }}
            />
          ))}
        </div>
      </div>
    );
  }
  const totUsed = entries.reduce((s, e) => s + e.usedBytes, 0);
  const totAll = entries.reduce((s, e) => s + e.totalBytes, 0);
  return (
    <div className="border-t border-slate-200 p-4">
      <div className="mb-1.5 flex justify-between text-xs text-slate-500">
        <span className="font-semibold text-slate-700">{fmtBytes(totUsed)} {t("used")}</span>
        <span>of {fmtBytes(totAll)}</span>
      </div>
      <div className="flex h-1.5 gap-0.5 overflow-hidden rounded-full bg-slate-100">
        {entries.map((e) => {
          const w = totAll > 0 ? (e.totalBytes / totAll) * 100 : 0;
          const fill = e.totalBytes > 0 ? (e.usedBytes / e.totalBytes) * 100 : 0;
          return (
            <span
              key={e.accountId}
              className="relative h-full overflow-hidden rounded-full"
              style={{ width: `${w}%`, background: providerDot(e.providerId), opacity: 0.25 }}
              title={`${e.providerId}`}
            >
              <span
                className="absolute inset-y-0 left-0"
                style={{ width: `${fill}%`, background: providerDot(e.providerId), opacity: 1 }}
              />
            </span>
          );
        })}
      </div>
    </div>
  );
}

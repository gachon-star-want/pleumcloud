import { useQuery } from "@tanstack/react-query";
import { api, type ProviderMeta } from "../api";
import ProviderLogo from "./ProviderLogo";

export default function ConnectPanel() {
  const providers = useQuery({
    queryKey: ["providers"],
    queryFn: api.providers,
  });
  const all = providers.data?.providers ?? [];
  const native = all.filter((p) => p.tier === "native");
  const experimental = all.filter((p) => p.tier === "experimental");

  return (
    <div className="mx-auto max-w-3xl">
      <section className="mb-8">
        <h3 className="mb-1 text-sm font-semibold uppercase tracking-wider text-slate-400">
          Full support
        </h3>
        <p className="mb-4 text-sm text-slate-500">
          One-click connect with official APIs. {native.length} clouds,{" "}
          {native.reduce((s, p) => s + p.freeTierGB, 0)}+ GB of free storage to
          combine.
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          {native.map((p) => (
            <ProviderCard key={p.id} p={p} />
          ))}
        </div>
      </section>

      <section>
        <h3 className="mb-1 text-sm font-semibold uppercase tracking-wider text-slate-400">
          Experimental
        </h3>
        <p className="mb-4 text-sm text-slate-500">
          Served through the rclone bridge — connect with credentials from each
          service.
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          {experimental.map((p) => (
            <ProviderCard key={p.id} p={p} />
          ))}
        </div>
      </section>
    </div>
  );
}

function ProviderCard({ p }: { p: ProviderMeta }) {
  const connect = () => {
    // Connector flows land in M2 (OAuth loopback / PAT paste / WebDAV form).
    alert(`Connecting ${p.name} arrives in milestone M2 🙂`);
  };

  return (
    <button
      onClick={connect}
      className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-4 text-left shadow-sm transition hover:border-blue-300 hover:shadow"
    >
      <span className="grid h-12 w-12 shrink-0 place-items-center">
        <ProviderLogo id={p.id} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-semibold">
          {p.name}
          {p.tier === "experimental" && (
            <span className="ml-2 rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-slate-400">
              beta
            </span>
          )}
        </span>
        <span className="block text-xs text-slate-500">
          {p.freeTierGB > 0 ? `${p.freeTierGB} GB free` : "bring your account"}
          {" · "}
          {authLabel(p.authKind)}
        </span>
      </span>
      <span className="text-sm font-semibold text-blue-600">Connect</span>
    </button>
  );
}

function authLabel(kind: ProviderMeta["authKind"]): string {
  switch (kind) {
    case "oauth2":
      return "1-click OAuth";
    case "pat":
      return "access token";
    case "webdav":
      return "WebDAV";
    default:
      return "credentials";
  }
}

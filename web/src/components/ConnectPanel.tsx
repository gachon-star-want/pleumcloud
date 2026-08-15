import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type ProviderMeta } from "../api";
import ProviderLogo from "./ProviderLogo";
import Modal from "./Modal";

type Dialog =
  | { kind: "pat"; provider: ProviderMeta }
  | { kind: "webdav" }
  | { kind: "creds"; provider: ProviderMeta }
  | { kind: "mediafire" }
  | null;

export default function ConnectPanel() {
  const [dialog, setDialog] = useState<Dialog>(null);
  const [error, setError] = useState<string | null>(null);
  const qc = useQueryClient();

  const providers = useQuery({ queryKey: ["providers"], queryFn: api.providers });
  const credentials = useQuery({ queryKey: ["credentials"], queryFn: api.credentials });
  const credMap = new Map(
    (credentials.data?.credentials ?? []).map((c) => [c.provider, c]),
  );

  const disconnect = useMutation({
    mutationFn: api.disconnect,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["accounts"] }),
  });

  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const connected = accounts.data?.accounts ?? [];

  const onConnect = (p: ProviderMeta) => {
    setError(null);
    if (!p.supported) {
      setError(`${p.name} connectors arrive in the next milestone.`);
      return;
    }
    switch (p.authKind) {
      case "oauth2": {
        const c = credMap.get(p.id);
        if (!c?.configured) {
          setDialog({ kind: "creds", provider: p });
        } else {
          window.location.href = `/api/connect/${p.id}/start`;
        }
        return;
      }
      case "pat":
        setDialog({ kind: "pat", provider: p });
        return;
      case "webdav":
        setDialog({ kind: "webdav" });
        return;
      default:
        setError(`${p.name} connects via the rclone bridge (coming soon).`);
    }
  };

  const all = providers.data?.providers ?? [];
  const native = all.filter((p) => p.tier === "native");
  const experimental = all.filter((p) => p.tier === "experimental");

  return (
    <div className="mx-auto max-w-3xl">
      {error && (
        <p className="mb-4 rounded-xl bg-amber-50 px-4 py-3 text-sm text-amber-700">
          {error}
        </p>
      )}

      {connected.length > 0 && (
        <section className="mb-8">
          <h3 className="mb-3 text-sm font-semibold uppercase tracking-wider text-slate-400">
            Connected ({connected.length})
          </h3>
          <div className="space-y-2">
            {connected.map((a) => {
              const meta = all.find((p) => p.id === a.providerId);
              return (
                <div
                  key={a.id}
                  className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-3"
                >
                  <ProviderLogo id={a.providerId} className="size-8" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-semibold">{a.label}</p>
                    <p className="text-xs text-slate-500">{meta?.name ?? a.providerId}</p>
                  </div>
                  <button
                    onClick={() => disconnect.mutate(a.id)}
                    className="rounded-lg px-2.5 py-1.5 text-xs font-semibold text-red-600 hover:bg-red-50"
                  >
                    Disconnect
                  </button>
                </div>
              );
            })}
          </div>
        </section>
      )}

      <section className="mb-8">
        <h3 className="mb-1 text-sm font-semibold uppercase tracking-wider text-slate-400">
          Full support
        </h3>
        <p className="mb-4 text-sm text-slate-500">
          One-click connect with official APIs.
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          {native.map((p) => (
            <ProviderCard key={p.id} p={p} onConnect={onConnect} />
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
            <ProviderCard key={p.id} p={p} onConnect={onConnect} />
          ))}
        </div>
      </section>

      {dialog?.kind === "pat" && (
        <PATDialog
          provider={dialog.provider}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "webdav" && <WebDAVDialog onClose={() => setDialog(null)} />}
      {dialog?.kind === "mediafire" && <MediaFireDialog onClose={() => setDialog(null)} />}
      {dialog?.kind === "creds" && (
        <CredsDialog provider={dialog.provider} onClose={() => setDialog(null)} />
      )}
    </div>
  );
}

function ProviderCard({
  p,
  onConnect,
}: {
  p: ProviderMeta;
  onConnect: (p: ProviderMeta) => void;
}) {
  const ready = p.supported;
  return (
    <button
      onClick={() => onConnect(p)}
      className={`flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-4 text-left shadow-sm transition ${
        ready ? "hover:border-blue-300 hover:shadow" : "opacity-60"
      }`}
    >
      <ProviderLogo id={p.id} className="size-11" />
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
      <span
        className={`text-sm font-semibold ${ready ? "text-blue-600" : "text-slate-400"}`}
      >
        {ready ? "Connect" : "Soon"}
      </span>
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

function Field({
  label,
  ...props
}: { label: string } & React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-semibold text-slate-500">{label}</span>
      <input
        {...props}
        className="w-full rounded-lg border border-slate-200 px-3 py-2 text-sm focus:border-blue-400 focus:outline-none"
      />
    </label>
  );
}

function ErrBanner({ error }: { error: string | null }) {
  if (!error) return null;
  return (
    <p className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p>
  );
}

function PATDialog({
  provider,
  onClose,
}: {
  provider: ProviderMeta;
  onClose: () => void;
}) {
  const [token, setToken] = useState("");
  const [email, setEmail] = useState("");
  const needsEmail = provider.id === "koofr";
  const [error, setError] = useState<string | null>(null);
  const qc = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => api.connectPAT(provider.id, token.trim(), undefined, needsEmail ? email.trim() : undefined),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["accounts"] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <Modal title={`Connect ${provider.name}`} onClose={onClose}>
      <div className="space-y-4">
        <p className="text-sm text-slate-500">
          Paste a personal access token.{" "}
          {provider.docsUrl && (
            <a
              href={provider.docsUrl}
              target="_blank"
              rel="noreferrer"
              className="font-medium text-blue-600 hover:underline"
            >
              How to get one ↗
            </a>
          )}
        </p>
        {needsEmail && (
          <Field
            label="Koofr account email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
            autoFocus
          />
        )}
        <Field
          label="API token"
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder={needsEmail ? "Settings → API tokens" : "mbx_pat_…"}
          autoFocus={!needsEmail}
        />
        <ErrBanner error={error} />
        <button
          disabled={token.trim() === "" || (needsEmail && email.trim() === "") || mutation.isPending}
          onClick={() => mutation.mutate()}
          className="w-full rounded-full bg-blue-600 py-2.5 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {mutation.isPending ? "Connecting…" : "Connect"}
        </button>
      </div>
    </Modal>
  );
}

function WebDAVDialog({ onClose }: { onClose: () => void }) {
  const [form, setForm] = useState({ url: "", username: "", password: "", label: "" });
  const [error, setError] = useState<string | null>(null);
  const qc = useQueryClient();
  const mutation = useMutation({
    mutationFn: () =>
      api.connectWebDAV(form.url.trim(), form.username, form.password, form.label.trim() || undefined),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["accounts"] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  return (
    <Modal title="Connect WebDAV" onClose={onClose}>
      <div className="space-y-4">
        <p className="text-sm text-slate-500">
          Works with Nextcloud, InfiniCLOUD, MagentaCloud, Koofr, any DAV
          server.
        </p>
        <Field label="Server URL" value={form.url} onChange={set("url")} placeholder="https://cloud.example.com/remote.php/dav/files/me/" autoFocus />
        <Field label="Username" value={form.username} onChange={set("username")} />
        <Field label="Password / app password" type="password" value={form.password} onChange={set("password")} />
        <Field label="Label (optional)" value={form.label} onChange={set("label")} placeholder="My Nextcloud" />
        <ErrBanner error={error} />
        <button
          disabled={form.url.trim() === "" || mutation.isPending}
          onClick={() => mutation.mutate()}
          className="w-full rounded-full bg-blue-600 py-2.5 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {mutation.isPending ? "Connecting…" : "Connect"}
        </button>
      </div>
    </Modal>
  );
}

function CredsDialog({
  provider,
  onClose,
}: {
  provider: ProviderMeta;
  onClose: () => void;
}) {
  const [form, setForm] = useState({ clientId: "", clientSecret: "" });
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  const save = useMutation({
    mutationFn: () => api.saveCredentials(provider.id, form.clientId.trim(), form.clientSecret.trim()),
    onSuccess: () => setSaved(true),
    onError: (e: Error) => setError(e.message),
  });

  return (
    <Modal title={`${provider.name} — one-time setup`} onClose={onClose}>
      <div className="space-y-4">
        <p className="text-sm text-slate-500">
          {provider.name} needs an OAuth app before accounts can connect. Paste
          your own app credentials (Client ID + secret); they stay on this
          machine.{" "}
          {provider.docsUrl && (
            <a
              href={provider.docsUrl}
              target="_blank"
              rel="noreferrer"
              className="font-medium text-blue-600 hover:underline"
            >
              Developer docs ↗
            </a>
          )}
        </p>
        <Field label="Client ID" value={form.clientId} onChange={set("clientId")} autoFocus />
        <Field label="Client secret" type="password" value={form.clientSecret} onChange={set("clientSecret")} />
        <ErrBanner error={error} />
        {saved ? (
          <button
            onClick={() => (window.location.href = `/api/connect/${provider.id}/start`)}
            className="w-full rounded-full bg-blue-600 py-2.5 text-sm font-semibold text-white hover:bg-blue-700"
          >
            Continue to {provider.name}
          </button>
        ) : (
          <button
            disabled={form.clientId.trim() === "" || save.isPending}
            onClick={() => save.mutate()}
            className="w-full rounded-full bg-blue-600 py-2.5 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {save.isPending ? "Saving…" : "Save"}
          </button>
        )}
      </div>
    </Modal>
  );
}


function MediaFireDialog({ onClose }: { onClose: () => void }) {
  const [form, setForm] = useState({ email: "", password: "", appId: "", apiKey: "" });
  const [error, setError] = useState<string | null>(null);
  const qc = useQueryClient();
  const mutation = useMutation({
    mutationFn: () =>
      api.connectMediaFire(form.email.trim(), form.password, form.appId.trim(), form.apiKey.trim()),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["accounts"] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));
  const ready = form.email.trim() && form.password && form.appId.trim() && form.apiKey.trim();

  return (
    <Modal title="Connect MediaFire" onClose={onClose}>
      <div className="space-y-4">
        <p className="text-sm text-slate-500">
          Create a free app in MediaFire Account Settings → Developers, then paste
          its credentials here.{" "}
          <a
            href="https://www.mediafire.com/developers/"
            target="_blank"
            rel="noreferrer"
            className="font-medium text-blue-600 hover:underline"
          >
            Developer docs ↗
          </a>
        </p>
        <Field label="Account email" type="email" value={form.email} onChange={set("email")} autoFocus />
        <Field label="Password" type="password" value={form.password} onChange={set("password")} />
        <Field label="Application ID" value={form.appId} onChange={set("appId")} />
        <Field label="API key" type="password" value={form.apiKey} onChange={set("apiKey")} />
        <ErrBanner error={error} />
        <button
          disabled={!ready || mutation.isPending}
          onClick={() => mutation.mutate()}
          className="w-full rounded-full bg-blue-600 py-2.5 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {mutation.isPending ? "Connecting…" : "Connect"}
        </button>
      </div>
    </Modal>
  );
}

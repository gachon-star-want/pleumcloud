// Typed client for the PleumCloud REST API.

export type AuthKind = "oauth2" | "pat" | "webdav" | "bridge";
export type Tier = "native" | "experimental";

export interface ProviderMeta {
  id: string;
  name: string;
  authKind: AuthKind;
  tier: Tier;
  freeTierGB: number;
  docsUrl?: string;
  maxUploadBytes?: number;
  /** A connector implementation is registered for this provider. */
  supported?: boolean;
}

export interface Account {
  id: string;
  providerId: string;
  label: string;
  createdAt: string;
  lastSyncedAt?: string;
}

export interface Health {
  status: string;
  version: string;
  time: string;
}

export interface Credential {
  provider: string;
  configured: boolean;
  hasByo: boolean;
  clientId?: string;
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${path}: ${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

async function send<T>(path: string, method: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error((data as { error?: string }).error ?? `${method} ${path}: ${res.status}`);
  }
  return data as T;
}

export const api = {
  health: () => get<Health>("/api/health"),
  providers: () => get<{ providers: ProviderMeta[] }>("/api/providers"),
  accounts: () => get<{ accounts: Account[] }>("/api/accounts"),
  credentials: () => get<{ credentials: Credential[] }>("/api/credentials"),

  connectPAT: (providerId: string, token: string, label?: string) =>
    send<{ id: string; label: string }>("/api/accounts", "POST", {
      providerId, method: "pat", token, label,
    }),

  connectWebDAV: (url: string, username: string, password: string, label?: string) =>
    send<{ id: string; label: string }>("/api/accounts", "POST", {
      providerId: "webdav", method: "webdav", url, username, password, label,
    }),

  disconnect: (id: string) => send<{ deleted: string }>(`/api/accounts/${id}`, "DELETE"),

  saveCredentials: (provider: string, clientId: string, clientSecret: string) =>
    send<{ status: string }>(`/api/credentials/${provider}`, "PUT", { clientId, clientSecret }),
};

/** Brand colors for provider badges. */
export const providerColor: Record<string, string> = {
  gdrive: "#4285f4",
  onedrive: "#0078d4",
  dropbox: "#0061ff",
  mybox: "#03c75a",
  drime: "#7c5cff",
  pcloud: "#ef7e33",
  koofr: "#38bdf8",
  webdav: "#64748b",
  infinicloud: "#14b8a6",
  mega: "#d9272e",
  box: "#0061d5",
  mediafire: "#1299f3",
  yandex: "#fc3f1d",
  hidrive: "#0f766e",
  jottacloud: "#2dd4bf",
  filen: "#3b82f6",
  internxt: "#111827",
  protondrive: "#6d4aff",
};

export function providerDot(id: string): string {
  return providerColor[id] ?? "#94a3b8";
}

import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { api } from "../api";
import { useT } from "../i18n";

/** Login/register gate shown in multi-user mode (cookie-based session). */
export default function AuthScreen({ onDone }: { onDone: () => void }) {
  const t = useT();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [form, setForm] = useState({ email: "", password: "" });
  const [error, setError] = useState<string | null>(null);

  const submit = useMutation({
    mutationFn: async () => {
      if (mode === "login") return api.login(form.email.trim(), form.password);
      return api.register(form.email.trim(), form.password);
    },
    onSuccess: onDone,
    onError: (e: Error) => setError(e.message),
  });

  return (
    <div className="grid min-h-full place-items-center p-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 text-center">
          <div className="text-4xl">☁️</div>
          <h1 className="mt-3 text-2xl font-bold">{t("welcome")}</h1>
          <p className="mt-1 text-sm text-slate-500">{t("welcomeDesc")}</p>
        </div>
        <div className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
          <div className="mb-4 grid grid-cols-2 rounded-xl bg-slate-100 p-1 text-sm font-semibold">
            {(["login", "register"] as const).map((m) => (
              <button
                key={m}
                onClick={() => { setMode(m); setError(null); }}
                className={`rounded-lg py-1.5 ${mode === m ? "bg-white text-blue-700 shadow-sm" : "text-slate-500"}`}
              >
                {t(m)}
              </button>
            ))}
          </div>
          <form
            className="space-y-3"
            onSubmit={(e) => { e.preventDefault(); if (form.email && form.password) submit.mutate(); }}
          >
            <input
              type="email" required autoFocus placeholder={t("email")} value={form.email}
              onChange={(e) => setForm((f) => ({ ...f, email: e.target.value }))}
              className="w-full rounded-xl border border-slate-200 px-3.5 py-2.5 text-sm focus:border-blue-400 focus:outline-none"
            />
            <input
              type="password" required placeholder={t("password")} value={form.password}
              onChange={(e) => setForm((f) => ({ ...f, password: e.target.value }))}
              className="w-full rounded-xl border border-slate-200 px-3.5 py-2.5 text-sm focus:border-blue-400 focus:outline-none"
            />
            {mode === "register" && (
              <p className="text-xs text-slate-400">{t("passwordHint")}</p>
            )}
            {error && <p className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p>}
            <button
              type="submit"
              disabled={submit.isPending}
              className="w-full rounded-full bg-blue-600 py-2.5 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {submit.isPending ? "…" : mode === "login" ? t("signIn") : t("signUp")}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, filesApi } from "../api";
import Modal from "./Modal";

/** Placement rules editor: priority-ordered, first match wins. */
export default function RulesDialog({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const rules = useQuery({ queryKey: ["rules"], queryFn: filesApi.rules });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({ field: "mime", op: "is", value: "", target: "", priority: 10 });

  const add = useMutation({
    mutationFn: () => filesApi.addRule({ ...form, priority: Number(form.priority) || 10 }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["rules"] }),
    onError: (e: Error) => setError(e.message),
  });
  const del = useMutation({
    mutationFn: (id: string) => filesApi.deleteRule(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["rules"] }),
  });

  const list = rules.data?.rules ?? [];
  const accts = accounts.data?.accounts ?? [];
  const accountName = (id: string) => accts.find((a) => a.id === id)?.label ?? id;

  return (
    <Modal title="Upload rules" onClose={onClose}>
      <div className="space-y-4">
        <p className="text-sm text-slate-500">
          New uploads match rules in priority order (lower number first); the
          first match decides the cloud. No match → most free space.
        </p>

        {list.length > 0 && (
          <div className="max-h-56 space-y-1.5 overflow-y-auto">
            {list.map((r) => (
              <div key={r.id} className="flex items-center gap-2 rounded-lg border border-slate-200 px-3 py-2 text-sm">
                <span className="w-8 shrink-0 text-xs text-slate-400">#{r.priority}</span>
                <span className="min-w-0 flex-1 truncate">
                  <b>{r.field}</b> {r.op} <b>{r.value}</b> → {accountName(r.target)}
                </span>
                <button
                  onClick={() => del.mutate(r.id)}
                  className="shrink-0 rounded px-1.5 text-slate-400 hover:text-red-600"
                  aria-label="Delete rule"
                >
                  ✕
                </button>
              </div>
            ))}
          </div>
        )}

        <div className="rounded-xl border border-dashed border-slate-300 p-3">
          <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-slate-400">Add rule</p>
          <div className="grid grid-cols-2 gap-2">
            <select
              value={form.field}
              onChange={(e) => setForm((f) => ({ ...f, field: e.target.value }))}
              className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-sm"
            >
              <option value="mime">mime</option>
              <option value="name">name</option>
              <option value="size">size</option>
            </select>
            <select
              value={form.op}
              onChange={(e) => setForm((f) => ({ ...f, op: e.target.value }))}
              className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-sm"
            >
              {form.field === "size" ? (
                <>
                  <option value="gt">greater than (bytes)</option>
                  <option value="lt">less than (bytes)</option>
                </>
              ) : (
                <>
                  <option value="is">starts with</option>
                  <option value="contains">contains</option>
                </>
              )}
            </select>
            <input
              value={form.value}
              onChange={(e) => setForm((f) => ({ ...f, value: e.target.value }))}
              placeholder={form.field === "mime" ? "video/" : form.field === "size" ? "1073741824" : "photo"}
              className="col-span-2 rounded-lg border border-slate-200 px-2.5 py-1.5 text-sm"
            />
            <select
              value={form.target}
              onChange={(e) => setForm((f) => ({ ...f, target: e.target.value }))}
              className="col-span-2 rounded-lg border border-slate-200 px-2.5 py-1.5 text-sm"
            >
              <option value="">— target cloud —</option>
              {accts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.label}
                </option>
              ))}
            </select>
            <input
              type="number"
              value={form.priority}
              onChange={(e) => setForm((f) => ({ ...f, priority: Number(e.target.value) }))}
              placeholder="priority"
              className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-sm"
            />
            <button
              disabled={!form.value || !form.target || add.isPending}
              onClick={() => add.mutate()}
              className="rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
            >
              Add
            </button>
          </div>
          {error && <p className="mt-2 rounded bg-red-50 px-2 py-1 text-xs text-red-700">{error}</p>}
        </div>
      </div>
    </Modal>
  );
}

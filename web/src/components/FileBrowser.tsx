import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { filesApi, fmtBytes, fmtDate, thumbURL, type FileRow } from "../api";
import { api, providerDot } from "../api";
import type { Crumb } from "../App";
import ProviderLogo from "./ProviderLogo";
import Modal from "./Modal";
import Preview from "./Preview";
import { useT } from "../i18n";

interface FileBrowserProps {
  onConnect: () => void;
  crumbs: Crumb[];
  onCrumbs: (crumbs: Crumb[]) => void;
  preview: FileRow | null;
  onPreview: (file: FileRow) => void;
  onClosePreview: () => void;
}

export default function FileBrowser({ onConnect, crumbs, onCrumbs, preview, onPreview, onClosePreview }: FileBrowserProps) {
  const t = useT();
  const [transferFile, setTransferFile] = useState<FileRow | null>(null);
  const [gallery, setGallery] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const qc = useQueryClient();
  const fileInput = useRef<HTMLInputElement>(null);

  const current = crumbs[crumbs.length - 1];
  const parentRemote = current?.remoteId ?? "";
  const parentAccount = crumbs.length > 0 ? current.accountId : "";

  const tree = useQuery({
    queryKey: ["tree", parentRemote, parentAccount],
    queryFn: () => filesApi.tree(parentRemote, parentAccount || undefined),
  });

  // Entering another folder invalidates the old selection.
  useEffect(() => {
    setSelectedId(null);
  }, [parentRemote, parentAccount]);

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["tree"] });
    qc.invalidateQueries({ queryKey: ["usage"] });
  };

  const upload = useMutation({
    mutationFn: (f: File) =>
      filesApi.upload(f, { parentAccount: parentAccount || undefined, parentRemote }),
    onSuccess: (d) => {
      setToast(`Uploaded to ${d.account ?? "cloud"}`);
      refresh();
    },
    onError: (e: Error) => setToast(e.message),
  });

  const act = useMutation({
    mutationFn: (body: Record<string, unknown>) => filesApi.op(body),
    onSuccess: () => refresh(),
    onError: (e: Error) => setToast(e.message),
  });

  const share = useMutation({
    mutationFn: (f: FileRow) => filesApi.share(f.id, true),
    onSuccess: (d) => {
      if (d.link) {
        navigator.clipboard?.writeText(d.link);
        setToast("Share link copied to clipboard");
      } else setToast("Link revoked");
    },
    onError: (e: Error) => setToast(e.message),
  });

  const files = tree.data?.files ?? [];
  const busy = tree.isLoading;
  const images = files.filter((f) => !f.isDir && (f.mime?.startsWith("image/") || /\.(jpe?g|png|gif|webp|bmp|heic|heif)$/i.test(f.name)));

  // Double-click (or Enter on the selection) opens: folders descend, files preview.
  const openRow = (f: FileRow) => {
    if (f.isDir) onCrumbs([...crumbs, { remoteId: f.remoteId, name: f.name, accountId: f.accountId }]);
    else onPreview(f);
  };

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.isContentEditable)) return;
      if (e.key === "Enter" && selectedId) {
        const f = files.find((x) => x.id === selectedId);
        if (f) openRow(f);
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  });

  const galleryGrid = (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
      {images.map((f) => (
        <button
          key={f.id}
          onClick={() => setSelectedId(f.id)}
          onDoubleClick={() => openRow(f)}
          className={`group relative overflow-hidden rounded-xl bg-slate-200 shadow-sm transition hover:shadow-md ${
            selectedId === f.id ? "ring-4 ring-blue-500 ring-offset-2" : ""
          }`}
        >
          <img src={thumbURL(f.id)} alt={f.name} loading="lazy" className="aspect-square w-full object-cover" />
          <span className="absolute inset-x-0 bottom-0 truncate bg-gradient-to-t from-black/70 to-transparent px-2 py-1.5 text-left text-xs font-medium text-white">
            {f.name}
          </span>
          <span
            className="absolute right-2 top-2 size-2.5 rounded-full ring-2 ring-white"
            style={{ background: providerDot(f.providerId) }}
          />
        </button>
      ))}
    </div>
  );

  return (
    <div className="mx-auto max-w-5xl">
      {toast && (
        <button
          onClick={() => setToast(null)}
          className="mb-3 w-full rounded-xl bg-slate-900 px-4 py-2.5 text-left text-sm text-white"
        >
          {toast} <span className="float-right opacity-60">✕</span>
        </button>
      )}

      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Breadcrumbs crumbs={crumbs} onNavigate={(i) => onCrumbs(crumbs.slice(0, i + 1))} />
        <div className="ml-auto flex gap-2">
          <button
            onClick={() => {
              const name = prompt("New folder name");
              if (name) act.mutate({ op: "mkdir", account: parentAccount || firstAccountId(files), parentRemote, name });
            }}
            className="rounded-full border border-slate-200 bg-white px-3.5 py-1.5 text-sm font-semibold text-slate-600 hover:border-blue-300"
          >
            {t("newFolder")}
          </button>
          {images.length > 0 && (
            <button
              onClick={() => setGallery((g) => !g)}
              title="Gallery view"
              className={`rounded-full border px-3.5 py-1.5 text-sm font-semibold ${
                gallery ? "border-blue-400 bg-blue-50 text-blue-700" : "border-slate-200 bg-white text-slate-600 hover:border-blue-300"
              }`}
            >
              {gallery ? t("list") : t("gallery")}
            </button>
          )}
          <button
            onClick={() => fileInput.current?.click()}
            disabled={upload.isPending}
            className="rounded-full bg-blue-600 px-4 py-1.5 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {upload.isPending ? t("uploading") : t("upload")}
          </button>
          <input
            ref={fileInput}
            type="file"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) upload.mutate(f);
              e.target.value = "";
            }}
          />
        </div>
      </div>

      {busy ? (
        <p className="py-16 text-center text-sm text-slate-400">{t("loading")}</p>
      ) : gallery && images.length > 0 ? (
        galleryGrid
      ) : files.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-300 bg-white py-16 text-center">
          <p className="font-semibold text-slate-600">{t("nothingHere")}</p>
          <p className="mt-1 text-sm text-slate-400">
            {crumbs.length === 0 ? t("connectPrompt") : t("dropOrFolder")}
          </p>
          {crumbs.length === 0 && (
            <button onClick={onConnect} className="mt-4 rounded-full bg-blue-600 px-5 py-2 text-sm font-semibold text-white">
              {t("connectCloud")}
            </button>
          )}
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-slate-200 bg-white">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-100 text-left text-xs uppercase tracking-wider text-slate-400">
                <th className="px-4 py-2.5 font-semibold">Name</th>
                <th className="hidden px-4 py-2.5 font-semibold sm:table-cell">Cloud</th>
                <th className="hidden px-4 py-2.5 font-semibold md:table-cell">Size</th>
                <th className="hidden px-4 py-2.5 font-semibold md:table-cell">Modified</th>
                <th className="px-4 py-2.5" />
              </tr>
            </thead>
            <tbody>
              {files.map((f) => (
                <tr
                  key={f.id}
                  onClick={() => setSelectedId(f.id)}
                  onDoubleClick={() => openRow(f)}
                  className={`group cursor-default border-b border-slate-50 last:border-0 ${
                    selectedId === f.id ? "bg-blue-50" : "hover:bg-slate-50"
                  }`}
                >
                  <td className="px-4 py-2.5">
                    <span className="flex max-w-xs items-center gap-2.5 text-left">
                      <span className="text-lg leading-none">{f.isDir ? "🗂️" : fileIcon(f)}</span>
                      <span className={`truncate font-medium ${selectedId === f.id ? "text-blue-700" : "text-slate-700"}`}>{f.name}</span>
                    </span>
                  </td>
                  <td className="hidden px-4 py-2.5 sm:table-cell">
                    <span className="inline-flex items-center gap-1.5 rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600">
                      <span className="size-2 rounded-full" style={{ background: providerDot(f.providerId) }} />
                      {f.accountLabel}
                    </span>
                  </td>
                  <td className="hidden px-4 py-2.5 text-slate-500 md:table-cell">{f.isDir ? "—" : fmtBytes(f.size)}</td>
                  <td className="hidden px-4 py-2.5 text-slate-500 md:table-cell">{fmtDate(f.mtime)}</td>
                  <td className="px-4 py-2.5 text-right">
                    <RowActions
                      file={f}
                      onShare={() => share.mutate(f)}
                      onDelete={() => {
                        if (confirm(`Delete ${f.name}?`)) act.mutate({ op: "delete", id: f.id });
                      }}
                      onRename={() => {
                        const name = prompt("New name", f.name);
                        if (name) act.mutate({ op: "rename", id: f.id, name });
                      }}
                      onTransfer={() => setTransferFile(f)}
                      onPreview={() => onPreview(f)}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {preview && <Preview file={preview} onClose={onClosePreview} />}
      {transferFile && (
        <TransferDialog file={transferFile} onClose={() => setTransferFile(null)} onQueued={(msg) => { setToast(msg); setTransferFile(null); }} />
      )}
    </div>
  );
}

function firstAccountId(files: FileRow[]): string {
  return files[0]?.accountId ?? "";
}

function fileIcon(f: FileRow): string {
  const m = f.mime ?? "";
  if (m.startsWith("image/")) return "🖼️";
  if (m.startsWith("video/")) return "🎬";
  if (m.startsWith("audio/")) return "🎵";
  if (m.includes("pdf")) return "📕";
  if (m.startsWith("text/")) return "📄";
  return "📦";
}

function Breadcrumbs({ crumbs, onNavigate }: { crumbs: Crumb[]; onNavigate: (i: number) => void }) {
  const t = useT();
  return (
    <nav className="flex items-center gap-1 text-sm">
      <button
        onClick={() => onNavigate(-1)}
        className={`rounded-lg px-2 py-1 font-semibold ${crumbs.length === 0 ? "text-blue-700" : "text-slate-500 hover:bg-slate-100"}`}
      >
        {t("allDrives")}
      </button>
      {crumbs.map((c, i) => (
        <span key={i} className="flex items-center gap-1">
          <span className="text-slate-300">/</span>
          <button
            onClick={() => onNavigate(i)}
            className={`rounded-lg px-2 py-1 ${i === crumbs.length - 1 ? "font-semibold text-blue-700" : "text-slate-500 hover:bg-slate-100"}`}
          >
            {c.name}
          </button>
        </span>
      ))}
    </nav>
  );
}

function RowActions({
  file,
  onShare,
  onDelete,
  onRename,
  onTransfer,
  onPreview,
}: {
  file: FileRow;
  onShare: () => void;
  onDelete: () => void;
  onRename: () => void;
  onTransfer: () => void;
  onPreview: () => void;
}) {
  const btn = "rounded-lg px-1.5 py-1 text-slate-400 opacity-0 transition group-hover:opacity-100 hover:bg-slate-200 hover:text-slate-700";
  return (
    <span className="inline-flex gap-0.5 whitespace-nowrap">
      {!file.isDir && (
        <button onClick={onPreview} title="Preview" className={btn}>
          👁
        </button>
      )}
      {!file.isDir && (
        <a href={`/api/file/${file.id}/download`} title="Download" className={btn}>
          ⬇
        </a>
      )}
      <button onClick={onShare} title="Share link" className={btn}>
        🔗
      </button>
      <button onClick={onTransfer} title="Copy to another cloud" className={btn}>
        ⇄
      </button>
      <button onClick={onRename} title="Rename" className={btn}>
        ✎
      </button>
      <button onClick={onDelete} title="Delete" className={btn}>
        🗑
      </button>
    </span>
  );
}

function TransferDialog({
  file,
  onClose,
  onQueued,
}: {
  file: FileRow;
  onClose: () => void;
  onQueued: (msg: string) => void;
}) {
  const qc = useQueryClient();
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const [error, setError] = useState<string | null>(null);
  const transfer = useMutation({
    mutationFn: (dst: string) => filesApi.transfer(file.id, dst),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["jobs"] });
      onQueued(`Transfer queued: ${file.name}`);
    },
    onError: (e: Error) => setError(e.message),
  });
  const targets = (accounts.data?.accounts ?? []).filter((a) => a.id !== file.accountId);

  return (
    <Modal title={`Copy “${file.name}” to…`} onClose={onClose}>
      <div className="space-y-2">
        {targets.length === 0 && (
          <p className="text-sm text-slate-500">Connect another cloud to transfer between them.</p>
        )}
        {targets.map((a) => (
          <button
            key={a.id}
            disabled={transfer.isPending}
            onClick={() => transfer.mutate(a.id)}
            className="flex w-full items-center gap-3 rounded-xl border border-slate-200 p-3 text-left hover:border-blue-300 disabled:opacity-50"
          >
            <ProviderLogo id={a.providerId} className="size-8" />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-semibold">{a.label}</span>
              <span className="block text-xs text-slate-400">runs in background</span>
            </span>
            <span className="text-sm font-semibold text-blue-600">Copy</span>
          </button>
        ))}
        {error && <p className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p>}
      </div>
    </Modal>
  );
}

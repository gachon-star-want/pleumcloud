import { lazy, Suspense, useEffect, useRef, useState, type ReactElement } from "react";
import { fmtBytes, inlineURL, providerDot, thumbURL, type FileRow } from "../api";
import { useT } from "../i18n";
import Fallback from "./preview/Fallback";

const DocxPreview = lazy(() => import("./preview/DocxPreview"));
const SheetPreview = lazy(() => import("./preview/SheetPreview"));

const DOCX_MIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
const XLSX_MIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";

const isDocx = (f: FileRow) => f.mime === DOCX_MIME || /\.docx$/i.test(f.name);
const isSheet = (f: FileRow) =>
  f.mime === XLSX_MIME || f.mime === "text/csv" || /\.(xlsx|csv)$/i.test(f.name);

/** Full-screen lightbox: images with zoom/pan, media, PDF, text, Office. */
export default function Preview({ file, onClose }: { file: FileRow; onClose: () => void }) {
  const t = useT();

  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [onClose]);

  const url = inlineURL(file.id);
  const m = file.mime ?? "";

  let body: ReactElement;
  if (m.startsWith("image/")) {
    body = <ImageViewer file={file} />;
  } else if (m.startsWith("video/")) {
    body = <video src={url} controls autoPlay className="max-h-full max-w-full rounded-xl" />;
  } else if (m.startsWith("audio/")) {
    body = (
      <div className="w-full max-w-xl rounded-2xl bg-white/5 p-10 text-center">
        <p className="mb-5 text-5xl">🎵</p>
        <audio src={url} controls autoPlay className="w-full" />
      </div>
    );
  } else if (m === "application/pdf") {
    body = <iframe src={url} title={file.name} className="h-full w-full rounded-xl border-0 bg-white" />;
  } else if (m.startsWith("text/") || m.startsWith("application/json")) {
    body = <iframe src={url} title={file.name} className="h-full w-full rounded-xl border-0 bg-white" />;
  } else if (isDocx(file)) {
    body = (
      <Suspense fallback={<p className="text-sm text-slate-300">{t("preparingPreview")}</p>}>
        <DocxPreview url={url} />
      </Suspense>
    );
  } else if (isSheet(file)) {
    body = (
      <Suspense fallback={<p className="text-sm text-slate-300">{t("preparingPreview")}</p>}>
        <SheetPreview url={url} />
      </Suspense>
    );
  } else {
    body = <Fallback url={url} mime={m} />;
  }

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-slate-950/95" role="dialog" aria-modal="true">
      <div className="flex shrink-0 items-center gap-3 px-4 py-2.5 text-white">
        <span className="size-2.5 shrink-0 rounded-full" style={{ background: providerDot(file.providerId) }} />
        <span className="min-w-0 truncate text-sm font-medium">{file.name}</span>
        <span className="hidden shrink-0 text-xs text-white/50 sm:inline">
          {fmtBytes(file.size)} · {file.accountLabel}
        </span>
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <a
            href={url.replace("inline=1", "inline=0")}
            title={t("download")}
            className="rounded-lg px-2 py-1 text-white/70 hover:bg-white/10 hover:text-white"
          >
            ⬇
          </a>
          <a
            href={url}
            target="_blank"
            rel="noreferrer"
            title={t("openOriginal")}
            className="rounded-lg px-2 py-1 text-white/70 hover:bg-white/10 hover:text-white"
          >
            ↗
          </a>
          <button
            onClick={onClose}
            title={t("close")}
            aria-label={t("close")}
            className="rounded-lg px-2 py-1 text-white/70 hover:bg-white/10 hover:text-white"
          >
            ✕
          </button>
        </div>
      </div>
      <div className="flex min-h-0 flex-1 items-center justify-center p-3 sm:p-6">{body}</div>
    </div>
  );
}

/** Image with fit/actual-size zoom, wheel zoom and drag-to-pan. Shows the
 *  1280px thumbnail instantly, then upgrades to the original bytes once the
 *  browser has them (HEIC stays on the server-transcoded JPEG pipeline). */
function ImageViewer({ file }: { file: FileRow }) {
  const t = useT();
  const isHEIC = /^image\/hei[cf]$/i.test(file.mime ?? "");
  const quick = thumbURL(file.id, 1280);
  const full = isHEIC ? thumbURL(file.id, 2048) : inlineURL(file.id);

  const [src, setSrc] = useState(quick);
  const [scale, setScale] = useState(1); // 1 = fit to viewport
  const [pos, setPos] = useState({ x: 0, y: 0 });
  const [dragging, setDragging] = useState(false);
  const imgRef = useRef<HTMLImageElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const dragStart = useRef<{ mx: number; my: number; px: number; py: number } | null>(null);

  useEffect(() => {
    let alive = true;
    const img = new Image();
    img.onload = () => alive && setSrc(full);
    img.src = full;
    return () => {
      alive = false;
    };
  }, [full]);

  const reset = () => {
    setScale(1);
    setPos({ x: 0, y: 0 });
  };

  // Back at fit (scale 1) there is nothing to pan — recenter.
  useEffect(() => {
    if (scale <= 1) setPos({ x: 0, y: 0 });
  }, [scale]);

  // Non-passive wheel listener: every wheel tick zooms (no page scroll here).
  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      setScale((s) => clamp(s * (e.deltaY < 0 ? 1.15 : 1 / 1.15)));
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, []);

  const actualSize = () => {
    const img = imgRef.current;
    if (!img || !img.naturalWidth || !scale) return reset();
    const fitWidth = img.getBoundingClientRect().width / scale;
    const ratio = img.naturalWidth / fitWidth;
    if (Math.abs(ratio - 1) < 0.02) return reset();
    setScale(ratio);
    setPos({ x: 0, y: 0 });
  };

  const onPointerDown = (e: React.PointerEvent) => {
    if (scale <= 1) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    dragStart.current = { mx: e.clientX, my: e.clientY, px: pos.x, py: pos.y };
    setDragging(true);
  };
  const onPointerMove = (e: React.PointerEvent) => {
    const d = dragStart.current;
    if (!d) return;
    setPos({ x: d.px + (e.clientX - d.mx), y: d.py + (e.clientY - d.my) });
  };
  const endDrag = () => {
    dragStart.current = null;
    setDragging(false);
  };

  return (
    <div
      ref={wrapRef}
      className="relative h-full w-full touch-none select-none overflow-hidden"
      style={{ cursor: scale > 1 ? (dragging ? "grabbing" : "grab") : "default" }}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onDoubleClick={() => (scale === 1 ? actualSize() : reset())}
    >
      <div className="grid h-full w-full place-items-center">
        <img
          ref={imgRef}
          src={src}
          alt={file.name}
          draggable={false}
          className="max-h-full max-w-full object-contain"
          style={{
            transform: `translate(${pos.x}px, ${pos.y}px) scale(${scale})`,
            transition: dragging ? "none" : "transform 0.15s ease-out",
          }}
        />
      </div>
      <div className="absolute bottom-3 left-1/2 flex -translate-x-1/2 items-center gap-0.5 rounded-full bg-black/60 px-2 py-1 text-white backdrop-blur">
        <button
          onClick={() => setScale((s) => clamp(s / 1.25))}
          className="rounded-full px-2 py-0.5 hover:bg-white/15"
          aria-label="Zoom out"
        >
          −
        </button>
        <span className="min-w-12 text-center text-xs tabular-nums">{Math.round(scale * 100)}%</span>
        <button
          onClick={() => setScale((s) => clamp(s * 1.25))}
          className="rounded-full px-2 py-0.5 hover:bg-white/15"
          aria-label="Zoom in"
        >
          +
        </button>
        <span className="mx-1 h-4 w-px bg-white/25" />
        <button onClick={reset} className="rounded-full px-2 py-0.5 text-xs hover:bg-white/15">
          {t("fit")}
        </button>
        <button onClick={actualSize} className="rounded-full px-2 py-0.5 text-xs hover:bg-white/15">
          1:1
        </button>
      </div>
    </div>
  );
}

function clamp(n: number): number {
  const v = Math.min(12, Math.max(1, n));
  return v;
}

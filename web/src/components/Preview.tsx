import { useEffect, type ReactElement } from "react";
import { fmtBytes, inlineURL, thumbURL, type FileRow } from "../api";
import Modal from "./Modal";

/** Full preview modal: images, video (seekable via Range), audio, PDF, text. */
export default function Preview({ file, onClose }: { file: FileRow; onClose: () => void }) {
  // ESC to close
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [onClose]);

  const url = inlineURL(file.id);
  const m = file.mime ?? "";
  let body: ReactElement;
  if (m.startsWith("image/")) {
    // HEIC is transcoded server-side (most browsers can't render the raw
    // bytes), so previews ride the JPEG thumbnail pipeline at a larger size.
    const src = /^image\/hei[cf]$/i.test(m) ? thumbURL(file.id, 1280) : url;
    body = <img src={src} alt={file.name} className="max-h-[70vh] max-w-full rounded-xl object-contain" />;
  } else if (m.startsWith("video/")) {
    body = <video src={url} controls autoPlay className="max-h-[70vh] max-w-full rounded-xl" />;
  } else if (m.startsWith("audio/")) {
    body = (
      <div className="w-full rounded-xl bg-slate-100 p-8 text-center">
        <p className="mb-4 text-4xl">🎵</p>
        <audio src={url} controls autoPlay className="w-full" />
      </div>
    );
  } else if (m === "application/pdf") {
    body = <iframe src={url} title={file.name} className="h-[70vh] w-full rounded-xl border-0" />;
  } else if (m.startsWith("text/") || m.startsWith("application/json")) {
    body = <TextPreview url={url} />;
  } else {
    body = (
      <div className="rounded-xl bg-slate-100 p-10 text-center">
        <p className="mb-2 text-4xl">📦</p>
        <p className="text-sm text-slate-500">No inline preview for {m || "this type"}.</p>
        <a href={url.replace("inline=1", "inline=0")} className="mt-3 inline-block text-sm font-semibold text-blue-600">
          Download instead
        </a>
      </div>
    );
  }
  return (
    <Modal title={file.name} onClose={onClose}>
      {body}
      <p className="mt-3 text-center text-xs text-slate-400">
        {fmtBytes(file.size)} · {file.accountLabel}
      </p>
    </Modal>
  );
}

function TextPreview({ url }: { url: string }) {
  return (
    <iframe src={url} title="text" className="h-[60vh] w-full rounded-xl border-0 bg-slate-50" />
  );
}

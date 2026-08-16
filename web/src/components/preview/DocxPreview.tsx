import { useEffect, useRef, useState } from "react";
import { renderAsync } from "docx-preview";
import { useT } from "../../i18n";
import Fallback from "./Fallback";

/** Read-only .docx rendering via docx-preview (lazy-loaded; renders pages
 *  much like Word does). Falls back to the download box on parse failure. */
export default function DocxPreview({ url }: { url: string }) {
  const t = useT();
  const host = useRef<HTMLDivElement>(null);
  const [failed, setFailed] = useState(false);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let alive = true;
    fetch(url)
      .then((r) => {
        if (!r.ok) throw new Error(`${r.status}`);
        return r.arrayBuffer();
      })
      .then((buf) => {
        if (!alive || !host.current) return;
        return renderAsync(buf, host.current, undefined, {
          inWrapper: true,
          ignoreWidth: false,
          ignoreHeight: false,
          breakPages: true,
        });
      })
      .then(() => alive && setReady(true))
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [url]);

  if (failed) return <Fallback url={url} />;
  return (
    <div className="flex h-full w-full flex-col overflow-hidden rounded-2xl bg-slate-800/60">
      {!ready && (
        <p className="px-4 py-2 text-center text-xs text-slate-300">{t("preparingPreview")}</p>
      )}
      <div ref={host} className="docx-host min-h-0 flex-1 overflow-auto" />
    </div>
  );
}

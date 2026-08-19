import { useEffect, useState } from "react";
import { api, openExternal, type Update } from "../api";
import { useT } from "../i18n";

// Manual-update banner (D13): compares this build against the latest
// GitHub release once per mount; the backend does the version compare.
// Dismissal lasts for the session (in-memory) — no state is persisted.
export default function UpdateBanner() {
  const t = useT();
  const [info, setInfo] = useState<Update | null>(null);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    api
      .update()
      .then((u) => setInfo(u.available ? u : null))
      .catch(() => {});
  }, []);

  if (!info || dismissed) return null;

  const releases = "https://github.com/gachon-star-want/pleumcloud/releases";
  const open = () => {
    openExternal(info.url ?? releases).then((opened) => {
      if (!opened) window.open(info.url ?? releases, "_blank", "noopener");
    });
  };

  return (
    <div className="mb-3 flex items-center gap-2 rounded-xl border border-blue-300 bg-blue-50 px-4 py-2.5 text-sm text-blue-800">
      <span className="min-w-0 flex-1">
        {t("updateAvailable").replace("{version}", info.latest)} —{" "}
        <button onClick={open} className="font-semibold hover:underline">
          {t("updateGet")}
        </button>
      </span>
      <button
        onClick={() => setDismissed(true)}
        aria-label={t("close")}
        className="shrink-0 rounded-lg px-2 py-1 opacity-60 transition hover:bg-blue-100 hover:opacity-100"
      >
        ✕
      </button>
    </div>
  );
}

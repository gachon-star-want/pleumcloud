import { useT } from "../../i18n";

/** Shown when a type can't be previewed inline (or a parser gave up). */
export default function Fallback({ url, mime = "" }: { url: string; mime?: string }) {
  const t = useT();
  return (
    <div className="rounded-2xl bg-white/5 px-10 py-12 text-center">
      <p className="mb-2 text-4xl">📦</p>
      <p className="text-sm text-slate-300">
        {t("noPreview")} {mime && <span className="text-slate-500">({mime})</span>}
      </p>
      <a
        href={url.replace("inline=1", "inline=0")}
        className="mt-4 inline-block rounded-full bg-blue-600 px-4 py-2 text-sm font-semibold text-white hover:bg-blue-700"
      >
        {t("downloadInstead")}
      </a>
    </div>
  );
}

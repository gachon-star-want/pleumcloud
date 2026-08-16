import { useEffect, useState } from "react";
import * as XLSX from "xlsx";
import { useT } from "../../i18n";
import Fallback from "./Fallback";

/** Read-only spreadsheet preview (.xlsx/.csv) via SheetJS, with sheet tabs. */
export default function SheetPreview({ url }: { url: string }) {
  const t = useT();
  const [book, setBook] = useState<XLSX.WorkBook | null>(null);
  const [sheet, setSheet] = useState<string>("");
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let alive = true;
    fetch(url)
      .then((r) => {
        if (!r.ok) throw new Error(`${r.status}`);
        return r.arrayBuffer();
      })
      .then((buf) => {
        if (!alive) return;
        const wb = XLSX.read(buf, { type: "array" });
        setBook(wb);
        setSheet(wb.SheetNames[0] ?? "");
      })
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [url]);

  if (failed) return <Fallback url={url} />;
  if (!book || !sheet) {
    return <p className="text-sm text-slate-300">{t("preparingPreview")}</p>;
  }
  const html = { __html: XLSX.utils.sheet_to_html(book.Sheets[sheet], { editable: false }) };
  return (
    <div className="flex h-full w-full flex-col overflow-hidden rounded-2xl bg-white">
      {book.SheetNames.length > 1 && (
        <div className="flex gap-1 overflow-x-auto border-b border-slate-200 px-2 py-1.5">
          {book.SheetNames.map((n) => (
            <button
              key={n}
              onClick={() => setSheet(n)}
              className={`whitespace-nowrap rounded-full px-3 py-1 text-xs font-semibold ${
                n === sheet ? "bg-blue-600 text-white" : "text-slate-500 hover:bg-slate-100"
              }`}
            >
              {n}
            </button>
          ))}
        </div>
      )}
      <div className="sheet-table min-h-0 flex-1 overflow-auto p-3" dangerouslySetInnerHTML={html} />
    </div>
  );
}

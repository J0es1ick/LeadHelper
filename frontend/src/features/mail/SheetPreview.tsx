import { useMemo, useState, type CSSProperties } from "react";
import SSF from "../../vendor/ssf/ssf.mjs";
export type CellStyle = {
  font?: string;
  size?: number;
  bold?: boolean;
  italic?: boolean;
  underline?: boolean;
  color?: string;
  fill?: string;
  align?: string;
  vertical?: string;
  wrap?: boolean;
  borders?: Record<string, string>;
  format?: string;
  formatId?: number;
};
export type Sheet = {
  hiddenRows?: number[];
  hidden?: boolean;
  name: string;
  rows: string[][];
  cells?: Record<string, { style: number; number?: number; formula?: string }>;
  widths?: Record<number, number>;
  heights?: Record<number, number>;
  merges?: { row: number; col: number; rows: number; cols: number }[];
  date1904?: boolean;
};
export type SpreadsheetData = { sheets?: Sheet[]; styles?: CellStyle[] };
export function columnName(n: number): string {
  return n < 26
    ? String.fromCharCode(65 + n)
    : columnName(Math.floor(n / 26) - 1) + String.fromCharCode(65 + (n % 26));
}
function css(s: CellStyle): CSSProperties {
  return {
    fontFamily: s.font
      ? `"${s.font.replace(/["\\]/g, "")}"`
      : "Arial, sans-serif",
    fontSize: s.size ? `${s.size}pt` : undefined,
    fontWeight: s.bold ? 700 : 400,
    fontStyle: s.italic ? "italic" : "normal",
    textDecoration: s.underline ? "underline" : undefined,
    color: s.color || "#17201b",
    backgroundColor: s.fill || "#fff",
    textAlign: ["left", "right", "center", "justify"].includes(s.align || "")
      ? (s.align as CSSProperties["textAlign"])
      : undefined,
    verticalAlign:
      s.vertical === "center"
        ? "middle"
        : ["top", "bottom"].includes(s.vertical || "")
          ? (s.vertical as CSSProperties["verticalAlign"])
          : "bottom",
    whiteSpace: s.wrap ? "pre-wrap" : "pre",
    borderTop: s.borders?.top,
    borderRight: s.borders?.right,
    borderBottom: s.borders?.bottom,
    borderLeft: s.borders?.left,
  };
}
export default function SheetPreview({
  sheets = [],
  styles = [],
}: SpreadsheetData) {
  const [selected, setSelected] = useState(0),
    [zoom, setZoom] = useState(100);
  const sheet = sheets[selected];
  const columns = Math.min(
    50,
    Math.max(
      0,
      ...(sheet?.rows.map((r) => r.length) || []),
      ...(sheet?.merges?.map((m) => m.col + m.cols) || []),
    ),
  );
  const merges = useMemo(() => {
    const starts = new Map<string, { rows: number; cols: number }>(),
      hidden = new Set<string>();
    for (const m of sheet?.merges || []) {
      if (
        m.rows * m.cols > 10000 ||
        starts.has(`${m.row}:${m.col}`) ||
        hidden.has(`${m.row}:${m.col}`)
      )
        continue;
      starts.set(`${m.row}:${m.col}`, m);
      for (let r = m.row; r < m.row + m.rows; r++)
        for (let c = m.col; c < m.col + m.cols; c++)
          if (r !== m.row || c !== m.col) hidden.add(`${r}:${c}`);
    }
    return { starts, hidden };
  }, [sheet]);
  const rowCount = Math.min(
    200,
    Math.max(
      sheet?.rows.length || 0,
      ...(sheet?.merges?.map((m) => m.row + m.rows) || []),
    ),
  );
  return (
    <>
      <div className="office-controls">
        <span className="muted">Только просмотр</span>
        <span className="spacer" />
        <label>
          Масштаб{" "}
          <select
            aria-label="Масштаб таблицы"
            value={zoom}
            onChange={(e) => setZoom(Number(e.target.value))}
          >
            {[75, 100, 125, 150].map((n) => (
              <option value={n} key={n}>
                {n}%
              </option>
            ))}
          </select>
        </label>
      </div>
      <div
        className="file-preview-tabs"
        role="tablist"
        aria-label="Листы документа"
      >
        {sheets.map((s, i) => (
          <button
            key={i}
            id={`file-sheet-${i}`}
            role="tab"
            aria-selected={selected === i}
            aria-controls="file-sheet-panel"
            className={selected === i ? "active" : ""}
            onClick={() => setSelected(i)}
          >
            {s.name}
          </button>
        ))}
      </div>
      <div
        className={"file-preview-table " + (sheet?.cells ? "office-sheet" : "")}
        role="tabpanel"
        id="file-sheet-panel"
        aria-labelledby={`file-sheet-${selected}`}
        tabIndex={0}
      >
        {rowCount ? (
          <table style={{ zoom: zoom / 100, tableLayout: "fixed" }}>
            <colgroup>
              <col style={{ width: 45 }} />
              {Array.from({ length: columns }, (_, c) => (
                <col key={c} style={{ width: sheet.widths?.[c] || 110 }} />
              ))}
            </colgroup>
            <thead>
              <tr>
                <th scope="col">№</th>
                {Array.from({ length: columns }, (_, c) => (
                  <th scope="col" key={c}>
                    {columnName(c)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {Array.from({ length: rowCount }, (_, r) => (
                <tr key={r} style={{ height: sheet.heights?.[r] || 24 }}>
                  <th scope="row">{r + 1}</th>
                  {Array.from({ length: columns }, (_, c) => {
                    const key = `${r}:${c}`;
                    if (merges.hidden.has(key)) return null;
                    const cell = sheet.cells?.[key],
                      style = styles[cell?.style || 0] || {},
                      merge = merges.starts.get(key);
                    let text = sheet.rows[r]?.[c] || "";
                    if (
                      cell?.number !== undefined &&
                      Number.isFinite(cell.number)
                    ) {
                      try {
                        text = SSF.format(
                          style.format || style.formatId || "General",
                          cell.number,
                          { date1904: sheet.date1904 },
                        );
                      } catch {}
                    }
                    return (
                      <td
                        key={c}
                        rowSpan={merge?.rows}
                        colSpan={merge?.cols}
                        style={
                          sheet.cells
                            ? {
                                ...css(style),
                                textAlign: style.align
                                  ? css(style).textAlign
                                  : cell?.number !== undefined
                                    ? "right"
                                    : "left",
                              }
                            : undefined
                        }
                        title={cell?.formula ? "=" + cell.formula : text}
                      >
                        {text}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <p className="file-preview-notice">Этот лист пуст.</p>
        )}
      </div>
    </>
  );
}

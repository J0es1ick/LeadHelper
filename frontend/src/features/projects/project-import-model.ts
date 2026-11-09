import type { Sheet, SpreadsheetData } from "../mail/SheetPreview";
import type { Project } from "./Projects";
import SSF from "../../vendor/ssf/ssf.mjs";

export const importFields = [
  ["name", "Название проекта"],
  ["status", "Текущий статус"],
  ["owner", "Ответственный"],
  ["startDate", "Дата начала"],
  ["dueDate", "Срок"],
  ["description", "Описание"],
  ["url", "Ссылка"],
] as const;
const guesses: Record<string, RegExp> = {
  name: /проект|наименование|название|клиент|project|name|customer/i,
  status: /статус|status|этап/i,
  owner: /ответствен|руководител|менеджер|owner|assignee/i,
  startDate: /начало|начала|start/i,
  dueDate: /срок|завершени|окончани|deadline|due/i,
  description: /описани|комментар|description/i,
  url: /ссылка|url|link/i,
};
export type ImportConfig = {
  includeHiddenRows: boolean;
  enabled: boolean;
  folder: string;
  header: number;
  mapping: Record<string, string>;
  mode: "rows" | "history" | "single";
  historyColumns: number[];
  historyDate: string;
  projectName: string;
  completed: string;
  skipSections: boolean;
  skipUnnamed: boolean;
};
export function displayCell(
  sheet: Sheet,
  row: number,
  col: number,
  book?: SpreadsheetData,
): string {
  const raw = sheet.rows[row]?.[col] || "";
  const cell = sheet.cells?.[`${row}:${col}`];
  const style = book?.styles?.[cell?.style || 0];
  if (cell?.number !== undefined && style?.format) {
    try {
      return SSF.format(style.format, cell.number, {
        date1904: sheet.date1904,
      });
    } catch {
      /* keep stored value */
    }
  }
  return raw;
}
function dateHeader(value: string): boolean {
  return /^(?:\d{1,2}[./]\d{1,2}(?:[./]\d{2,4})?|\d{4}-\d{2}-\d{2})(?:\s|$)/.test(
    value.trim(),
  );
}
export function guessConfig(
  sheet: Sheet,
  book: SpreadsheetData,
  header?: number,
): ImportConfig {
  let best = header ?? 0;
  if (header === undefined) {
    let score = -1;
    sheet.rows.slice(0, 20).forEach((row, index) => {
      const next = importFields.reduce(
        (sum, [key]) => sum + (row.some((v) => guesses[key].test(v)) ? 1 : 0),
        0,
      );
      if (next > score) {
        score = next;
        best = index;
      }
    });
  }
  const headers = (sheet.rows[best] || []).map((_, i) =>
    displayCell(sheet, best, i, book),
  );
  const mapping = Object.fromEntries(
    importFields.map(([key]) => {
      const i = headers.findIndex((v) => guesses[key].test(v));
      return [key, i < 0 ? "" : String(i)];
    }),
  );
  const historyColumns = headers.flatMap((v, i) => (dateHeader(v) ? [i] : []));
  const dateCol = headers.findIndex((v) => /^(дата|date)$/i.test(v.trim()));
  return {
    includeHiddenRows: false,
    enabled:
      !sheet.hidden && sheet.rows.some((row) => row.some((v) => v.trim())),
    folder: sheet.name.trim(),
    header: best,
    mapping,
    mode:
      historyColumns.length >= 2
        ? "history"
        : mapping.name === "" && dateCol >= 0 && mapping.status !== ""
          ? "single"
          : "rows",
    historyColumns,
    historyDate: dateCol < 0 ? "" : String(dateCol),
    projectName: sheet.name.trim(),
    completed:
      "Завершён, Завершен, Завершена, Завершено, Закрыт, Закрыта, Отменён, Отменен, Done, Closed, Completed",
    skipSections: false,
    skipUnnamed: false,
  };
}
function importDate(sheet: Sheet, r: number, col: string): string {
  if (col === "") return "";
  const value = (sheet.rows[r]?.[Number(col)] || "").trim();
  if (!value) return "";
  const numeric = sheet.cells?.[`${r}:${col}`]?.number;
  if (numeric !== undefined)
    return SSF.format("yyyy-mm-dd", numeric, { date1904: sheet.date1904 });
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
  const m = value.match(/^(\d{1,2})[./](\d{1,2})[./](\d{4})$/);
  if (m) return `${m[3]}-${m[2].padStart(2, "0")}-${m[1].padStart(2, "0")}`;
  throw Error(`Проверьте дату в строке ${r + 1}: ${value}`);
}
export function prepareImport(
  book: SpreadsheetData,
  configs: ImportConfig[],
  fileName: string,
) {
  const batches: { projects: Partial<Project>[]; fields: string[] }[] = [];
  let skipped = 0,
    total = 0;
  (book.sheets || []).forEach((sheet, index) => {
    const c = configs[index];
    if (!c?.enabled) return;
    const fail = (message: string): never => {
      throw Error(`Лист «${sheet.name}»: ${message}`);
    };
    if (
      c.mode !== "single" &&
      (c.mapping.name === "" || c.mapping.name === undefined)
    )
      fail("выберите столбец с названием проекта");
    if (
      c.mode === "single" &&
      (!c.projectName.trim() || c.mapping.status === "")
    )
      fail("укажите название проекта и столбец статуса");
    if (c.mode === "history" && !c.historyColumns.length)
      fail("выберите столбцы истории статусов");
    const completed = new Set(
      c.completed
        .split(/[,;\n]/)
        .map((v) => v.trim().toLowerCase())
        .filter(Boolean),
    );
    const state = (status: string) =>
      completed.has(status.toLowerCase()) ? "completed" : "active";
    const projects: Partial<Project>[] = [];
    const singleHistory: NonNullable<Project["history"]> = [];
    const hiddenRows = new Set(sheet.hiddenRows || []);
    for (let r = c.header + 1; r < sheet.rows.length; r++) {
      const row = sheet.rows[r];
      if (!row.some((v) => v.trim())) continue;
      if (!c.includeHiddenRows && hiddenRows.has(r)) {
        skipped++;
        continue;
      }
      const value = (key: string) =>
        c.mapping[key] !== "" && c.mapping[key] !== undefined
          ? (row[Number(c.mapping[key])] || "").trim()
          : "";
      if (c.mode === "single") {
        const text = value("status");
        if (text)
          singleHistory.push({
            key: `r${r}`,
            label:
              c.historyDate !== ""
                ? displayCell(sheet, r, Number(c.historyDate), book) ||
                  `Строка ${r + 1}`
                : `Строка ${r + 1}`,
            text,
          });
        continue;
      }
      const name = value("name");
      const style = sheet.cells?.[`${r}:${c.mapping.name}`]?.style;
      if (
        c.skipSections &&
        name &&
        row.filter((v) => v.trim()).length === 1 &&
        style !== undefined &&
        book.styles?.[style]?.bold
      ) {
        skipped++;
        continue;
      }
      if (!name) {
        if (c.skipUnnamed) {
          skipped++;
          continue;
        }
        fail(
          `в строке ${r + 1} нет названия проекта; сопоставьте столбец или включите пропуск строк без названия`,
        );
      }
      const history =
        c.mode === "history"
          ? [...c.historyColumns]
              .sort((a, b) => a - b)
              .flatMap((col) => {
                const text = (row[col] || "").trim();
                return text
                  ? [
                      {
                        key: `c${col}`,
                        label:
                          displayCell(sheet, c.header, col, book) ||
                          `Столбец ${col + 1}`,
                        text,
                      },
                    ]
                  : [];
              })
          : undefined;
      const status = value("status") || history?.at(-1)?.text || "В работе";
      try {
        projects.push({
          name,
          group: c.folder.trim(),
          status,
          state: state(status),
          owner: value("owner"),
          description: value("description"),
          url: value("url"),
          startDate: importDate(sheet, r, c.mapping.startDate || ""),
          dueDate: importDate(sheet, r, c.mapping.dueDate || ""),
          source: fileName,
          history,
        });
      } catch (e) {
        fail((e as Error).message);
      }
    }
    if (c.mode === "single" && singleHistory.length) {
      const status = singleHistory.at(-1)!.text;
      projects.push({
        name: c.projectName.trim(),
        group: c.folder.trim(),
        status,
        state: state(status),
        history: singleHistory,
        source: fileName,
      });
    }
    if (!projects.length)
      fail(
        "нет строк с проектами; снимите выбор листа или проверьте настройки",
      );
    total += projects.length;
    if (total > 2000)
      fail(
        "за один импорт можно сохранить до 2000 проектов; выберите меньше листов или разделите файл",
      );
    const fields: string[] =
      c.mode === "single"
        ? ["name", "status", "history", "group"]
        : [
            "group",
            ...importFields
              .filter(
                ([k]) => c.mapping[k] !== "" && c.mapping[k] !== undefined,
              )
              .map(([k]) => k),
          ];
    if (c.mode === "history") fields.push("history", "status");
    batches.push({ projects, fields });
  });
  if (!batches.length) throw Error("Выберите хотя бы один лист");
  if (
    new TextEncoder().encode(JSON.stringify({ batches })).length >
    6 * 1024 * 1024
  )
    throw Error(
      "Слишком много текста за один импорт. Выберите меньше листов или разделите файл.",
    );
  return { batches, skipped, total };
}

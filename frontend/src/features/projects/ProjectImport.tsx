import { useState } from "react";
import { api } from "../../shared/api";
import { Modal } from "../../shared/ui";
import type { SpreadsheetData } from "../mail/SheetPreview";
import type { Project } from "./Projects";
import {
  displayCell,
  guessConfig,
  importFields,
  prepareImport,
  type ImportConfig,
} from "./project-import-model";

export function ProjectImport({
  close,
  saved,
}: {
  close: () => void;
  saved: () => void;
}) {
  const [book, setBook] = useState<SpreadsheetData | null>(null),
    [fileName, setFileName] = useState("");
  const [configs, setConfigs] = useState<ImportConfig[]>([]),
    [selected, setSelected] = useState(0),
    [merge, setMerge] = useState(true);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [prepared, setPrepared] = useState<Project[] | null>(null),
    [requestID, setRequestID] = useState(""),
    [page, setPage] = useState(1),
    [skipped, setSkipped] = useState(0);
  const sheet = book?.sheets?.[selected],
    config = configs[selected];
  const headers =
    sheet && config
      ? Array.from(
          {
            length: Math.max(
              0,
              ...sheet.rows
                .slice(config.header, config.header + 6)
                .map((r) => r.length),
            ),
          },
          (_, i) => displayCell(sheet, config.header, i, book!),
        )
      : [];
  const update = (patch: Partial<ImportConfig>) => {
    setConfigs((v) =>
      v.map((c, i) => (i === selected ? { ...c, ...patch } : c)),
    );
    setError("");
  };
  async function load(file?: File) {
    if (!file) return;
    setError("");
    setPrepared(null);
    setBook(null);
    setConfigs([]);
    if (file.size > 5 * 1024 * 1024) {
      setError("Максимальный размер таблицы — 5 МБ");
      return;
    }
    setBusy(true);
    try {
      const data = await new Promise<string>((resolve, reject) => {
        const r = new FileReader();
        r.onload = () => resolve(String(r.result).split(",")[1]);
        r.onerror = () => reject(Error("Не удалось прочитать файл"));
        r.readAsDataURL(file);
      });
      const parsed = await api<SpreadsheetData>(
        "/projects/import/parse",
        "POST",
        { name: file.name, data },
      );
      setBook(parsed);
      setConfigs((parsed.sheets || []).map((s) => guessConfig(s, parsed)));
      setFileName(file.name);
      setSelected(0);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function preview() {
    if (!book) return;
    setBusy(true);
    setError("");
    try {
      const result = prepareImport(book, configs, fileName);
      const projects = await api<Project[]>(
        "/projects/import/preview-batch",
        "POST",
        { batches: result.batches, merge },
      );
      setPrepared(projects);
      setSkipped(result.skipped);
      setPage(1);
      setRequestID(crypto.randomUUID());
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function commit() {
    if (!prepared || busy) return;
    setBusy(true);
    setError("");
    try {
      await api("/projects/import", "POST", {
        projects: prepared,
        requestId: requestID,
      });
      saved();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const columns = (
    <>
      <option value="">Не импортировать</option>
      {headers.map((h, i) => (
        <option key={i} value={i}>
          {i + 1}. {h || "Без названия"}
        </option>
      ))}
    </>
  );
  return (
    <Modal title="Импорт проектов" onClose={close} wide>
      <div className="project-import">
        <p>
          Выберите листы и сопоставьте столбцы. Каждый лист станет папкой с его
          названием; папку можно переименовать. XLSX, CSV или TSV до 5 МБ, до
          2000 проектов за импорт.
        </p>
        {!prepared ? (
          <fieldset disabled={busy}>
            <label className="project-file">
              Таблица
              <input
                type="file"
                accept=".xlsx,.csv,.tsv"
                onChange={(e) => void load(e.target.files?.[0])}
              />
            </label>
            {sheet && config && (
              <>
                <div
                  className="project-import-sheets"
                  aria-label="Листы для импорта"
                >
                  {(book?.sheets || []).map((s, i) => (
                    <div key={i} className={selected === i ? "selected" : ""}>
                      <input
                        type="checkbox"
                        aria-label={"Импортировать лист " + s.name}
                        checked={configs[i].enabled}
                        onChange={(e) =>
                          setConfigs((v) =>
                            v.map((c, j) =>
                              j === i ? { ...c, enabled: e.target.checked } : c,
                            ),
                          )
                        }
                      />
                      <button
                        type="button"
                        onClick={() => setSelected(i)}
                        aria-pressed={selected === i}
                      >
                        {s.name}
                        {s.hidden ? " (скрытый)" : ""}
                        <small>{s.rows.length} строк</small>
                      </button>
                    </div>
                  ))}
                </div>
                <h3>Настройки листа «{sheet.name}»</h3>
                {!config.enabled && (
                  <p>
                    Этот лист исключён из импорта. Его настройки сохраняются при
                    переключении.
                  </p>
                )}
                <div className="project-form-row">
                  <label>
                    Папка
                    <input
                      aria-label="Папка листа"
                      maxLength={300}
                      value={config.folder}
                      onChange={(e) => update({ folder: e.target.value })}
                      placeholder="Без папки"
                    />
                  </label>
                  <label>
                    Строка заголовков
                    <input
                      aria-label="Строка заголовков"
                      type="number"
                      min={1}
                      max={Math.max(1, sheet.rows.length)}
                      value={config.header + 1}
                      onChange={(e) => {
                        const header = Math.max(
                          0,
                          Math.min(
                            sheet.rows.length - 1,
                            Number(e.target.value) - 1,
                          ),
                        );
                        const guessed = guessConfig(sheet, book!, header);
                        update({
                          header,
                          mapping: guessed.mapping,
                          historyColumns: guessed.historyColumns,
                        });
                      }}
                    />
                  </label>
                </div>
                <label>
                  Структура листа
                  <select
                    aria-label="Структура листа"
                    value={config.mode}
                    onChange={(e) =>
                      update({ mode: e.target.value as ImportConfig["mode"] })
                    }
                  >
                    <option value="rows">Одна строка — один проект</option>
                    <option value="history">
                      Проекты в строках, история статусов в столбцах
                    </option>
                    <option value="single">
                      Весь лист — один проект, история в строках
                    </option>
                  </select>
                </label>
                {config.mode === "single" && (
                  <div className="project-form-row">
                    <label>
                      Название проекта
                      <input
                        value={config.projectName}
                        onChange={(e) =>
                          update({ projectName: e.target.value })
                        }
                      />
                    </label>
                    <label>
                      Дата / подпись записи
                      <select
                        value={config.historyDate}
                        onChange={(e) =>
                          update({ historyDate: e.target.value })
                        }
                      >
                        {columns}
                      </select>
                    </label>
                  </div>
                )}
                <div className="project-mapping">
                  {importFields
                    .filter(
                      ([key]) => config.mode !== "single" || key === "status",
                    )
                    .map(([key, title]) => (
                      <label key={key}>
                        {title}
                        {key === "name" ? " *" : ""}
                        <select
                          aria-label={title + " — столбец"}
                          value={config.mapping[key] || ""}
                          onChange={(e) =>
                            update({
                              mapping: {
                                ...config.mapping,
                                [key]: e.target.value,
                              },
                            })
                          }
                        >
                          {columns}
                        </select>
                      </label>
                    ))}
                </div>
                {config.mode === "history" && (
                  <details className="project-history-columns" open>
                    <summary>
                      Столбцы истории · {config.historyColumns.length}
                    </summary>
                    <p>
                      Последний заполненный столбец справа задаёт текущий
                      статус, если отдельный столбец статуса не выбран.
                    </p>
                    <div>
                      {headers.map((h, i) => (
                        <label className="project-check" key={i}>
                          <input
                            type="checkbox"
                            checked={config.historyColumns.includes(i)}
                            onChange={(e) =>
                              update({
                                historyColumns: e.target.checked
                                  ? [...config.historyColumns, i]
                                  : config.historyColumns.filter(
                                      (c) => c !== i,
                                    ),
                              })
                            }
                          />
                          {i + 1}. {h || "Без названия"}
                        </label>
                      ))}
                    </div>
                  </details>
                )}
                <details className="project-import-sample">
                  <summary>Первые строки выбранного листа</summary>
                  <div className="project-import-table">
                    <table>
                      <thead>
                        <tr>
                          {headers.map((h, i) => (
                            <th key={i}>
                              {i + 1}. {h}
                            </th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {sheet.rows
                          .slice(config.header + 1, config.header + 6)
                          .map((row, r) => (
                            <tr key={r}>
                              {headers.map((_, col) => (
                                <td key={col}>{row[col] || ""}</td>
                              ))}
                            </tr>
                          ))}
                      </tbody>
                    </table>
                  </div>
                </details>
                <label>
                  Статусы завершённых проектов
                  <input
                    value={config.completed}
                    onChange={(e) => update({ completed: e.target.value })}
                  />
                  <small>
                    Точные названия через запятую. Оставьте пустым, чтобы все
                    проекты были активными.
                  </small>
                </label>
                {!!sheet.hiddenRows?.length && (
                  <label className="project-check">
                    <input
                      type="checkbox"
                      checked={config.includeHiddenRows}
                      onChange={(e) =>
                        update({ includeHiddenRows: e.target.checked })
                      }
                    />
                    Включить скрытые строки листа. По умолчанию они
                    пропускаются.
                  </label>
                )}
                {config.mode !== "single" && (
                  <>
                    <label className="project-check">
                      <input
                        type="checkbox"
                        checked={config.skipSections}
                        onChange={(e) =>
                          update({ skipSections: e.target.checked })
                        }
                      />
                      Пропускать заголовки разделов: жирное название без других
                      значений в строке.
                    </label>
                    <label className="project-check">
                      <input
                        type="checkbox"
                        checked={config.skipUnnamed}
                        onChange={(e) =>
                          update({ skipUnnamed: e.target.checked })
                        }
                      />
                      Пропускать строки без названия проекта.
                    </label>
                  </>
                )}
                <label className="project-check">
                  <input
                    type="checkbox"
                    checked={merge}
                    onChange={(e) => setMerge(e.target.checked)}
                  />
                  Обновлять совпадения по папке и названию. Невыбранные поля
                  сохранятся.
                </label>
                <p>
                  Выбрано листов: {configs.filter((c) => c.enabled).length}.
                  Файл импортируется как снимок; повторная загрузка обновит
                  проекты. Для фоновой синхронизации подключите Google Таблицу.
                </p>
                <button
                  className="button primary"
                  disabled={busy || !configs.some((c) => c.enabled)}
                  onClick={() => void preview()}
                >
                  Проверить импорт
                </button>
              </>
            )}
          </fieldset>
        ) : (
          <>
            <p>
              <strong>
                Будет создано: {prepared.filter((p) => !p.version).length} ·
                обновлено: {prepared.filter((p) => p.version).length} · папок:{" "}
                {new Set(prepared.map((p) => p.group || "")).size}
              </strong>
              {skipped > 0 && (
                <>
                  <br />
                  Пропущено скрытых и служебных строк: {skipped}
                </>
              )}
            </p>
            <div className="project-import-table">
              <table>
                <thead>
                  <tr>
                    <th>Действие</th>
                    <th>Папка / проект</th>
                    <th>Статус</th>
                    <th>Состояние</th>
                    <th>Ответственный</th>
                    <th>Срок</th>
                  </tr>
                </thead>
                <tbody>
                  {prepared.slice((page - 1) * 50, page * 50).map((p) => (
                    <tr key={p.id}>
                      <td>{p.version ? "Обновление" : "Создание"}</td>
                      <td>
                        <small>{p.group || "Без папки"}</small>
                        <br />
                        {p.name}
                        {!!p.history?.length && (
                          <small> · {p.history.length} записей истории</small>
                        )}
                      </td>
                      <td>
                        <div className="project-preview-status">{p.status}</div>
                      </td>
                      <td>
                        <select
                          aria-label={"Раздел проекта " + p.name}
                          value={p.state}
                          disabled={busy}
                          onChange={(e) => {
                            const state = e.target.value;
                            setPrepared(
                              (items) =>
                                items?.map((item) =>
                                  item.id === p.id ? { ...item, state } : item,
                                ) || null,
                            );
                            setRequestID(crypto.randomUUID());
                          }}
                        >
                          <option value="active">Активные</option>
                          <option value="completed">Завершённые</option>
                          <option value="archived">Архив</option>
                        </select>
                      </td>
                      <td>{p.owner || "—"}</td>
                      <td>{p.dueDate || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <nav
              className="project-pagination"
              aria-label="Страницы предпросмотра"
            >
              <button
                className="button"
                disabled={busy || page === 1}
                onClick={() => setPage((v) => v - 1)}
              >
                Назад
              </button>
              <span>
                {page} / {Math.ceil(prepared.length / 50)}
              </span>
              <button
                className="button"
                disabled={busy || page * 50 >= prepared.length}
                onClick={() => setPage((v) => v + 1)}
              >
                Далее
              </button>
            </nav>
            <footer>
              <button
                className="button"
                disabled={busy}
                onClick={() => setPrepared(null)}
              >
                Назад к столбцам
              </button>
              <button
                className="button primary"
                disabled={busy}
                onClick={() => void commit()}
              >
                Сохранить {prepared.length} проектов
              </button>
            </footer>
          </>
        )}
        {busy && <p role="status">Обрабатываем таблицу…</p>}
        {error && (
          <p className="inline-warning" role="alert">
            {error}
          </p>
        )}
      </div>
    </Modal>
  );
}

import { useEffect, useRef, useState } from "react";
import { Download, Plus, X } from "lucide-react";
import { useData } from "../../app/store";
import { api, APIError, dayLabel, endpoint, shiftDay } from "../../shared/api";
import { poll } from "../../shared/live";
import { Modal } from "../../shared/ui";
import { exportPeriods, type PeriodOptions } from "./export-periods";
import "./worklog-export.css";

type Status = {
  id: string;
  state: "running" | "ready" | "error" | "cancelled";
  done: number;
  chunks: number;
  employees: number;
  missing: number;
  error?: string;
};
const weekdays = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];
export function WorklogExport({
  week,
  close,
}: {
  week: string;
  close: () => void;
}) {
  const { state } = useData();
  const members = state.members.filter((m) => !m.inactive);
  const [options, setOptions] = useState<PeriodOptions>({
    from: week,
    to: shiftDay(week, 6),
    split: "none",
    boundary: "start",
    weekdays: [1, 4],
    dates: [],
  });
  const [selected, setSelected] = useState(() => members.map((m) => m.id));
  const [search, setSearch] = useState("");
  const [date, setDate] = useState("");
  const [status, setStatus] = useState<Status | null>(null);
  const [starting, setStarting] = useState(false),
    [downloading, setDownloading] = useState(false),
    [error, setError] = useState("");
  const mounted = useRef(true),
    job = useRef<Status | null>(null);
  const downloadAbort = useRef<AbortController | null>(null);
  job.current = status;
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      downloadAbort.current?.abort();
      if (job.current?.state === "running")
        void api(
          `/jira/worklog-exports/${endpoint(job.current.id)}`,
          "DELETE",
        ).catch(() => {});
    };
  }, []);
  useEffect(() => {
    if (!status || status.state !== "running") return;
    const c = new AbortController();
    poll(
      async () => {
        try {
          const value = await api<Status>(
            `/jira/worklog-exports/${endpoint(status.id)}`,
            "GET",
            undefined,
            c.signal,
          );
          if (!c.signal.aborted) {
            setStatus(value);
            setError("");
          }
        } catch (e) {
          if (!c.signal.aborted) {
            if (e instanceof APIError && e.status === 404)
              setStatus(
                (v) =>
                  v && {
                    ...v,
                    state: "error",
                    error:
                      "Выгрузка больше недоступна. Сервер мог перезапуститься; сформируйте отчёт заново.",
                  },
              );
            else setError((e as Error).message);
          }
          throw e;
        }
      },
      { signal: c.signal, interval: 1000 },
    );
    return () => c.abort();
  }, [status?.id, status?.state]);
  let periods: ReturnType<typeof exportPeriods> = [],
    validation = "";
  try {
    periods = exportPeriods(options);
  } catch (e) {
    validation = (e as Error).message;
  }
  const busy = starting || status?.state === "running";
  function change(values: Partial<PeriodOptions>) {
    setOptions((v) => ({ ...v, ...values }));
    setStatus(null);
    setError("");
  }
  async function start() {
    if (busy || validation || !selected.length) return;
    setStarting(true);
    setStatus(null);
    setError("");
    try {
      const value = await api<Status>("/jira/worklog-exports", "POST", {
        ...options,
        memberIds: selected,
      });
      if (!mounted.current) {
        await api(`/jira/worklog-exports/${endpoint(value.id)}`, "DELETE");
        return;
      }
      setStatus(value);
    } catch (e) {
      if (mounted.current) setError((e as Error).message);
    } finally {
      if (mounted.current) setStarting(false);
    }
  }
  async function cancel() {
    if (!status) return;
    try {
      await api(`/jira/worklog-exports/${endpoint(status.id)}`, "DELETE");
      setStatus((v) => v && { ...v, state: "cancelled" });
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function download() {
    if (!status || status.state !== "ready" || downloading) return;
    const c = new AbortController();
    downloadAbort.current = c;
    setDownloading(true);
    setError("");
    const timeout = setTimeout(() => c.abort(), 30000);
    try {
      const response = await fetch(
        `/api/jira/worklog-exports/${endpoint(status.id)}/file`,
        { cache: "no-store", signal: c.signal },
      );
      if (!response.ok) {
        const value = await response.json();
        throw Error(value.error || "Не удалось скачать отчёт");
      }
      const blob = await response.blob();
      if (!mounted.current) return;
      const url = URL.createObjectURL(blob),
        a = document.createElement("a");
      a.href = url;
      a.download = `Часы_${options.from}_${options.to}.xlsx`;
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 10000);
    } catch (e) {
      if (mounted.current)
        setError(
          (e as Error).name === "AbortError"
            ? "Не удалось дождаться файла. Повторите скачивание."
            : (e as Error).message,
        );
    } finally {
      clearTimeout(timeout);
      if (mounted.current) setDownloading(false);
    }
  }
  const shown = members.filter((m) =>
    m.name.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
  );
  return (
    <Modal
      title="Выгрузка часов в Excel"
      onClose={close}
      wide
      className="worklog-export-modal"
    >
      <div className="worklog-export">
        <p className="muted">
          Фактические списания Jira по сотрудникам команды. Даты — по Москве;
          обе границы периода включены.
        </p>
        <fieldset disabled={!!busy || downloading}>
          <div className="export-dates">
            <label>
              Начало периода
              <input
                type="date"
                value={options.from}
                onChange={(e) => change({ from: e.target.value })}
              />
            </label>
            <label>
              Конец периода
              <input
                type="date"
                value={options.to}
                onChange={(e) => change({ to: e.target.value })}
              />
            </label>
          </div>
          <label>
            Разделить часы
            <select
              aria-label="Разделить часы"
              value={options.split}
              onChange={(e) =>
                change({ split: e.target.value as PeriodOptions["split"] })
              }
            >
              <option value="none">Весь период целиком</option>
              <option value="weekly">По дням недели</option>
              <option value="dates">По своим датам</option>
            </select>
          </label>
          {options.split !== "none" && (
            <label>
              Контрольная дата
              <select
                aria-label="Контрольная дата"
                value={options.boundary}
                onChange={(e) =>
                  change({ boundary: e.target.value as "start" | "end" })
                }
              >
                <option value="start">Начинает новый подпериод</option>
                <option value="end">Завершает текущий подпериод</option>
              </select>
            </label>
          )}
          {options.split === "weekly" && (
            <div
              className="export-weekdays"
              role="group"
              aria-label="Контрольные дни недели"
            >
              {weekdays.map((name, i) => (
                <button
                  type="button"
                  key={name}
                  aria-pressed={options.weekdays.includes(i + 1)}
                  onClick={() =>
                    change({
                      weekdays: options.weekdays.includes(i + 1)
                        ? options.weekdays.filter((d) => d !== i + 1)
                        : [...options.weekdays, i + 1],
                    })
                  }
                >
                  {name}
                </button>
              ))}
            </div>
          )}
          {options.split === "dates" && (
            <div className="export-custom-dates">
              <div>
                <input
                  aria-label="Добавить контрольную дату"
                  type="date"
                  min={options.from}
                  max={options.to}
                  value={date}
                  onChange={(e) => setDate(e.target.value)}
                />
                <button
                  type="button"
                  className="button small"
                  disabled={
                    !date ||
                    date < options.from ||
                    date > options.to ||
                    options.dates.includes(date)
                  }
                  onClick={() => {
                    change({ dates: [...options.dates, date].sort() });
                    setDate("");
                  }}
                >
                  <Plus size={16} />
                  Добавить
                </button>
              </div>
              <div className="export-date-tags">
                {options.dates.map((value) => (
                  <button
                    type="button"
                    key={value}
                    onClick={() =>
                      change({
                        dates: options.dates.filter((d) => d !== value),
                      })
                    }
                    aria-label={`Удалить дату ${value}`}
                  >
                    {dayLabel(value, {
                      day: "numeric",
                      month: "short",
                      year: "numeric",
                    })}
                    <X size={14} />
                  </button>
                ))}
              </div>
            </div>
          )}
          <details className="export-members">
            <summary>
              Сотрудники: {selected.length} из {members.length}
            </summary>
            <div className="export-member-tools">
              <input
                aria-label="Поиск сотрудников для выгрузки"
                placeholder="Найти сотрудника"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
              <button
                type="button"
                className="text-button"
                onClick={() => {
                  setSelected(members.map((m) => m.id));
                  setStatus(null);
                }}
              >
                Выбрать всех
              </button>
              <button
                type="button"
                className="text-button"
                onClick={() => {
                  setSelected([]);
                  setStatus(null);
                }}
              >
                Снять выбор
              </button>
            </div>
            <div className="export-member-list">
              {shown.map((m) => (
                <label key={m.id}>
                  <input
                    type="checkbox"
                    checked={selected.includes(m.id)}
                    onChange={(e) => {
                      setSelected((ids) =>
                        e.target.checked
                          ? [...ids, m.id]
                          : ids.filter((id) => id !== m.id),
                      );
                      setStatus(null);
                    }}
                  />
                  <span>
                    {m.name}
                    {(!m.jiraSite || !m.jiraUserId || !m.jiraLogin) && (
                      <small>Без сопоставления с Jira</small>
                    )}
                  </span>
                </label>
              ))}
            </div>
          </details>
        </fieldset>
        {validation ? (
          <p className="inline-warning" role="alert">
            {validation}
          </p>
        ) : (
          <section
            className="export-period-preview"
            aria-label="Подпериоды отчёта"
          >
            <strong>
              {periods.length === 1
                ? "Один итог за период"
                : `Подпериодов: ${periods.length}`}
            </strong>
            <p>Каждый отрезок считается с нуля. Дни не пересекаются.</p>
            <ol>
              {periods.map((p) => (
                <li key={p.from}>
                  {dayLabel(p.from, {
                    day: "2-digit",
                    month: "2-digit",
                    year: "numeric",
                  })}{" "}
                  —{" "}
                  {dayLabel(p.to, {
                    day: "2-digit",
                    month: "2-digit",
                    year: "numeric",
                  })}
                </li>
              ))}
            </ol>
          </section>
        )}
        {busy && (
          <div className="export-progress" role="status">
            <progress max={status?.chunks || 1} value={status?.done || 0} />
            <span>
              {starting
                ? "Начинаем загрузку…"
                : `Загружаем списания Jira: ${status?.done} из ${status?.chunks} частей периода`}
            </span>
            <small>
              Большие периоды загружаются частями. Закрытие окна отменяет
              выгрузку.
            </small>
          </div>
        )}
        {status?.state === "ready" && (
          <div className="export-ready" role="status">
            Отчёт готов: сотрудников — {status.employees}, подпериодов —{" "}
            {periods.length}.
            {status.missing > 0 && (
              <p>
                Без данных: {status.missing}. В Excel они отмечены отдельно;
                часы оставлены пустыми.
              </p>
            )}
          </div>
        )}
        {status?.state === "cancelled" && (
          <p role="status">Выгрузка отменена.</p>
        )}
        {(error || status?.error) && (
          <p className="inline-warning" role="alert">
            {error || status?.error}
          </p>
        )}
        <div className="export-actions">
          <button className="button" onClick={close}>
            Закрыть
          </button>
          {status?.state === "ready" && <button className="button" disabled={downloading} onClick={() => void start()}>Обновить данные</button>}
          {status?.state === "running" ? (
            <button className="button" onClick={() => void cancel()}>
              Отменить загрузку
            </button>
          ) : status?.state === "ready" ? (
            <button
              className="button primary"
              disabled={downloading}
              onClick={() => void download()}
            >
              <Download size={17} />
              {downloading ? "Скачиваем…" : "Скачать Excel"}
            </button>
          ) : (
            <button
              className="button primary"
              disabled={
                !!busy ||
                !!validation ||
                !selected.length ||
                selected.length > 500
              }
              onClick={() => void start()}
            >
              <Download size={17} />
              Сформировать Excel
            </button>
          )}
        </div>
      </div>
    </Modal>
  );
}

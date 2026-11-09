import { useEffect, useRef, useState } from "react";
import { poll, unchanged } from "../../shared/live";
import { Download, FileBarChart, RefreshCw } from "lucide-react";
import {
  api,
  dateKey,
  dayLabel,
  download,
  shiftDay,
  weekStart,
} from "../../shared/api";
import { useData } from "../../app/store";
import { Empty, Source } from "../../shared/ui";
import { statuses, type Report, type Task } from "../../shared/types";
export function Reports({ edit }: { edit: (t: Task) => void }) {
  const { state, notify } = useData();
  const [from, setFrom] = useState(weekStart());
  const [to, setTo] = useState(shiftDay(weekStart(), 6));
  const [source, setSource] = useState("all");
  const [report, setReport] = useState<Report | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const query = new URLSearchParams({ from, to, source }).toString();
  const loaded = useRef("");
  useEffect(() => {
    const c = new AbortController();
    const key = `${query}:${state.settings.demo}`;
    setBusy(loaded.current !== key);
    setError("");
    if (loaded.current !== key) setReport(null);
    poll(
      () =>
        api<Report>("/reports?" + query, "GET", undefined, c.signal)
          .then((v) => {
            if (!c.signal.aborted) {
              setReport((old) => unchanged(old, v));
              loaded.current = key;
              setError("");
            }
          })
          .catch((e) => {
            if (!c.signal.aborted) {
              setError(e.message);
            }
            throw e;
          })
          .finally(() => {
            if (!c.signal.aborted) setBusy(false);
          }),
      { signal: c.signal, interval: 15000 },
    );
    return () => {
      c.abort();
    };
  }, [query, state.revision, state.settings.demo]);
  function markdown() {
    if (!report) return;
    download(
      "team-report.md",
      `# Отчёт команды\n\nПериод: ${from} — ${to}\nИсточник: ${source}\nРежим: ${state.settings.demo ? "демонстрация" : "рабочие данные"}\n\n- Задач за период: ${report.total}\n- Завершено в периоде: ${report.completed}\n- В работе / на проверке: ${report.inProgress}\n- Просрочено на ${dateKey()}: ${report.overdue}\n- Оценка задач периода: ${report.plannedHours} ч\n\n## Задачи\n\n` +
        report.tasks
          .map(
            (t) =>
              `- [${t.status === "done" ? "x" : " "}] ${t.title} — ${statuses[t.status]}, ${t.estimate} ч, ${t.source}`,
          )
          .join("\n"),
      "text/markdown",
    );
    notify("Отчёт выгружен");
  }
  return (
    <>
      <div className="page-heading heading-flex">
        <div>
          <div className="eyebrow">ДАННЫЕ ДЛЯ РЕШЕНИЙ</div>
          <h1>Отчёты команды</h1>
          <p>Понятная картина работы без ручного сбора по сервисам.</p>
        </div>
        <div className="button-row">
          <button
            className="button"
            disabled={!report || busy}
            onClick={markdown}
          >
            <FileBarChart size={16} />
            Markdown
          </button>
          <a
            className={"button primary " + (!report || busy ? "disabled" : "")}
            href={"/api/reports/export?" + query}
          >
            <Download size={16} />
            Выгрузить CSV
          </a>
        </div>
      </div>
      <div className="toolbar report-toolbar">
        <label>
          С
          <input
            type="date"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          />
        </label>
        <label>
          По
          <input
            type="date"
            value={to}
            onChange={(e) => setTo(e.target.value)}
          />
        </label>
        <select
          value={source}
          onChange={(e) => setSource(e.target.value)}
          aria-label="Источник отчёта"
        >
          <option value="all">Все источники</option>
          <option value="local">LeadHelper</option>
          {state.settings.modules.jira && <option value="jira">Jira</option>}
        </select>
        <button
          className="button small"
          onClick={() => {
            setFrom(weekStart());
            setTo(shiftDay(weekStart(), 6));
          }}
        >
          Эта неделя
        </button>
        {busy && <RefreshCw size={17} className="spin" />}
      </div>
      {error && <div className="inline-warning">{error}</div>}
      {report && (
        <>
          <div className="stats-grid">
            {[
              {
                name: "Задач в периоде",
                value: report.total,
                note: "По дате плана, сроку или завершению",
              },
              {
                name: "Завершено",
                value: report.completed,
                note: "Дата завершения попадает в период",
              },
              {
                name: "Просрочено",
                value: report.overdue,
                note: "Незавершённые с истёкшим сроком",
              },
              {
                name: "Объём работ",
                value: report.plannedHours + " ч",
                note: "Сумма оценок задач в отчёте",
              },
            ].map((v) => (
              <div className="stat-card" key={v.name}>
                <span>{v.name}</span>
                <strong>{v.value}</strong>
                <small>{v.note}</small>
              </div>
            ))}
          </div>
          <div className="report-charts">
            <section className="panel">
              <div className="section-heading">
                <h2>Распределение по статусам</h2>
                <span className="count-badge">{report.total}</span>
              </div>
              <div className="status-chart">
                {Object.entries(statuses).map(([key, name]) => {
                  const count = report.tasks.filter(
                    (t) => t.status === key,
                  ).length;
                  return (
                    <div key={key}>
                      <span>
                        <i className={"status-dot " + key} />
                        {name}
                      </span>
                      <div className="chart-bar">
                        <i
                          className={key}
                          style={{
                            width:
                              (report.total
                                ? (count / report.total) * 100
                                : 0) + "%",
                          }}
                        />
                      </div>
                      <strong>{count}</strong>
                    </div>
                  );
                })}
              </div>
            </section>
            <section className="panel">
              <div className="section-heading">
                <h2>Источники задач</h2>
              </div>
              <div className="source-chart">
                {["local", "jira", "bitrix"]
                  .filter((s) => s === "local" || state.settings.modules[s])
                  .map((s) => {
                    const count = report.tasks.filter(
                      (t) => t.source === s,
                    ).length;
                    return (
                      <div key={s}>
                        <Source value={s} />
                        <strong>{count}</strong>
                        <div className="progress-line">
                          <span
                            style={{
                              width:
                                (report.total
                                  ? (count / report.total) * 100
                                  : 0) + "%",
                            }}
                          />
                        </div>
                      </div>
                    );
                  })}
              </div>
            </section>
          </div>
          <section className="panel">
            <div className="section-heading">
              <div>
                <h2>Детализация</h2>
                <p>
                  {dayLabel(from)} — {dayLabel(to)} ·{" "}
                  {state.settings.demo
                    ? "Демонстрационные данные"
                    : "Данные рабочего пространства"}
                </p>
              </div>
            </div>
            {report.tasks.length ? (
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Задача</th>
                      <th>Источник</th>
                      <th>Статус</th>
                      <th>Оценка</th>
                      <th>Срок</th>
                    </tr>
                  </thead>
                  <tbody>
                    {report.tasks.map((t) => (
                      <tr key={t.id}>
                        <td>
                          <button
                            className="table-title"
                            onClick={() => edit(t)}
                          >
                            {t.title}
                          </button>
                          <small>{t.project}</small>
                        </td>
                        <td>
                          <Source value={t.source} />
                        </td>
                        <td>
                          <span className={"status-pill " + t.status}>
                            {statuses[t.status]}
                          </span>
                        </td>
                        <td>{t.estimate} ч</td>
                        <td>{dayLabel(t.dueDate)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <Empty
                title="Нет задач за этот период"
                description="Выберите другой период или запланируйте задачи команды."
              />
            )}
          </section>
          <div className="info-strip margin-top">
            В отчёт входят задачи, выбранные в плане на дни периода, задачи со
            сроком в периоде и завершённые в нём. Загрузка — часы из плана.
            Личные задачи исключены. Отчёт отражает текущее состояние,
            исторические снимки пока не ведутся.
          </div>
        </>
      )}
    </>
  );
}

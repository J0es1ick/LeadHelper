import { useEffect, useRef, useState } from "react";
import { Search, X, Check } from "lucide-react";
import { useData } from "../../app/store";
import { api, dayLabel, endpoint } from "../../shared/api";
import { Modal, Empty } from "../../shared/ui";
import {
  statuses,
  type Member,
  type PlannedTask,
  type Task,
  type TeamDayPlan,
} from "../../shared/types";
import { TaskPagination, useDebounced } from "../../shared/taskPages";
import { poll, unchanged } from "../../shared/live";
import { JiraIssueEditor } from "../integrations/JiraIssueEditor";
export function DayPlan({
  member,
  date,
  onClose,
}: {
  member: Member;
  date: string;
  onClose: () => void;
}) {
  const { act, state } = useData();
  const [createJira, setCreateJira] = useState(false);
  const [search, setSearch] = useState("");
  const term = useDebounced(search);
  const [page, setPage] = useState(1),
    [reload, setReload] = useState(0),
    [data, setData] = useState<TeamDayPlan | null>(null);
  const [selected, setSelected] = useState<Record<string, PlannedTask>>({});
  const revision = useRef<string | null>(null);
  const [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false);
  const path = `/team/day/${endpoint(member.id)}/${date}`;
  useEffect(() => {
    setPage(1);
  }, [term]);
  useEffect(() => {
    const c = new AbortController();
    setLoading(true);
    setError("");
    poll(
      () =>
        api<TeamDayPlan>(
          `${path}?page=${page}&limit=25&search=${endpoint(term)}`,
          "GET",
          undefined,
          c.signal,
        )
          .then((v) => {
            if (c.signal.aborted) return;
            setData((old) => unchanged(old, v));
            setError("");
            setPage(v.candidates.page);
            if (revision.current === null) {
              revision.current = v.revision;
              setSelected(
                Object.fromEntries(v.selected.map((e) => [e.taskId, e])),
              );
            }
          })
          .catch((e) => {
            if (!c.signal.aborted) setError(e.message);
            throw e;
          })
          .finally(() => {
            if (!c.signal.aborted) setLoading(false);
          }),
      { signal: c.signal, interval: 10000 },
    );
    return () => c.abort();
  }, [path, term, page, reload]);
  const entries = Object.values(selected);
  const hours = entries.reduce((n, e) => n + e.hours, 0);
  function toggle(t: Task) {
    setSelected((old) => {
      const next = { ...old };
      if (next[t.id]) delete next[t.id];
      else
        next[t.id] = {
          memberId: member.id,
          date,
          taskId: t.id,
          hours: 0,
          task: t,
        };
      return next;
    });
  }
  async function save() {
    setBusy(true);
    setError("");
    try {
      await act(
        () =>
          api(path, "PUT", {
            revision: revision.current,
            entries: entries.map(({ task: _, ...entry }) => entry),
          }),
        "План на день сохранён",
      );
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось сохранить план");
    } finally {
      setBusy(false);
    }
  }
  if (createJira)
    return (
      <JiraIssueEditor
        member={member}
        onClose={() => {
          setCreateJira(false);
          setReload((n) => n + 1);
        }}
        onCreated={(t) => {
          if (
            t.assigneeId === member.id &&
            ["todo", "progress"].includes(t.status)
          )
            toggle(t);
        }}
      />
    );
  return (
    <Modal
      title={"План на " + dayLabel(date, { day: "numeric", month: "long" })}
      onClose={() => {
        if (!busy) onClose();
      }}
      wide
      className="day-plan-modal"
    >
      <div className="day-plan">
        <div className="day-plan-person">
          <strong>{member.name}</strong>
          <span>
            Выберите задачи этого сотрудника: «К выполнению» и «В работе».
          </span>
          {state.settings.modules.jira && !state.settings.demo && (
            <button
              className="button small"
              disabled={busy}
              onClick={() => setCreateJira(true)}
            >
              Создать задачу в Jira
            </button>
          )}
        </div>
        <div className="day-plan-columns">
          <section className="day-plan-selected">
            <h3>
              В плане · {entries.length}
              <span>{hours ? `${hours} ч` : "Часы не указаны"}</span>
            </h3>
            <div className="day-plan-selection-scroll">
              {entries.length ? (
                entries.map((e) => (
                  <div className="day-plan-selection" key={e.taskId}>
                    <div>
                      <strong>{e.task.title}</strong>
                      <small>
                        {e.task.externalId || e.task.project} ·{" "}
                        {statuses[e.task.status]}
                        {e.task.assigneeId !== member.id
                          ? " · исполнитель изменился"
                          : ""}
                      </small>
                    </div>
                    <label>
                      Часы на день
                      <input
                        type="number"
                        min={0}
                        max={24}
                        step="0.5"
                        placeholder="—"
                        aria-label={"Часы: " + e.task.title}
                        value={e.hours || ""}
                        disabled={busy}
                        onChange={(event) => {
                          const hours = Math.max(
                            0,
                            Math.min(24, Number(event.target.value)),
                          );
                          setSelected((old) => ({
                            ...old,
                            [e.taskId]: { ...old[e.taskId], hours },
                          }));
                        }}
                      />
                    </label>
                    <button
                      className="icon-button"
                      aria-label={"Убрать из плана: " + e.task.title}
                      disabled={busy}
                      onClick={() => toggle(e.task)}
                    >
                      <X size={16} />
                    </button>
                  </div>
                ))
              ) : (
                <p className="muted">
                  Отметьте нужные задачи в списке. Можно выбрать несколько; часы
                  необязательны.
                </p>
              )}
            </div>
          </section>
          <section className="day-plan-available">
            <div className="search-field">
              <Search size={17} />
              <input
                aria-label="Поиск задач сотрудника"
                placeholder="Найти задачу по названию, ключу или проекту…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            {error && (
              <div className="inline-warning" role="alert">
                {error}
                <button
                  className="button small"
                  disabled={busy}
                  onClick={() => {
                    revision.current = null;
                    setReload((n) => n + 1);
                  }}
                >
                  Перезагрузить план
                </button>
              </div>
            )}
            <div className="day-plan-candidates" aria-busy={loading}>
              {loading ? (
                <p className="muted pad" role="status">
                  Загрузка задач…
                </p>
              ) : (
                data?.candidates.items.map((t) => (
                  <label
                    className={
                      "day-plan-option " + (selected[t.id] ? "chosen" : "")
                    }
                    key={t.id}
                  >
                    <input
                      type="checkbox"
                      checked={!!selected[t.id]}
                      disabled={
                        busy || (!selected[t.id] && entries.length >= 200)
                      }
                      onChange={() => toggle(t)}
                    />
                    <span>
                      <strong>{t.title}</strong>
                      <small>
                        {t.externalId || t.project} · {statuses[t.status]}
                        {t.estimate ? ` · оценка задачи ${t.estimate} ч` : ""}
                      </small>
                    </span>
                    {selected[t.id] && <Check size={16} />}
                  </label>
                ))
              )}
              {!loading && data?.candidates.total === 0 && (
                <Empty
                  title="Подходящих задач нет"
                  description={
                    term
                      ? "Попробуйте другой запрос."
                      : "Здесь появятся задачи сотрудника со статусами «К выполнению» и «В работе» после синхронизации Jira или создания в бэклоге."
                  }
                />
              )}
            </div>
            <TaskPagination
              data={data?.candidates ?? null}
              onPage={setPage}
              disabled={loading || busy}
            />
          </section>
        </div>
        <p className="muted day-plan-hint">
          Одна задача может входить в план нескольких дней. Часы на день
          задаются отдельно от оценки Jira.
        </p>
        <div className="modal-footer">
          <button className="button" disabled={busy} onClick={onClose}>
            Отмена
          </button>
          <button
            className="button primary"
            disabled={busy || revision.current === null}
            onClick={() => void save()}
          >
            {busy ? "Сохранение…" : "Сохранить план"}
          </button>
        </div>
      </div>
    </Modal>
  );
}

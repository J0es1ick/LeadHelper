import { useEffect, useRef, useState } from "react";
import { poll, unchanged } from "../../shared/live";
import { api, dayLabel, endpoint, shiftDay } from "../../shared/api";
import { Modal, Empty } from "../../shared/ui";
import { TaskPagination } from "../../shared/taskPages";
import { JiraIssueEditor } from "../integrations/JiraIssueEditor";
import type { Member } from "../../shared/types";

export type WorklogWeek = {
  cells: {
    memberId: string;
    date: string;
    seconds: number;
    count: number;
    taskCount: number;
    tasks: {
      issueKey: string;
      title: string;
      seconds: number;
      count: number;
    }[];
  }[];
  syncedAt?: string;
  warning?: string;
  timezone: string;
};
type Worklog = {
  id: string;
  issueKey: string;
  title: string;
  url: string;
  started: string;
  seconds: number;
  comment?: string;
};
type WorklogPage = {
  items: Worklog[];
  page: number;
  pages: number;
  total: number;
  limit: number;
};
export const hoursText = (hours: number) =>
  hours.toLocaleString("ru-RU", { maximumFractionDigits: 2 });
export function useWorklogs(week: string, enabled: boolean, identity: string) {
  const loaded = useRef("");
  const [data, setData] = useState<WorklogWeek>({
    cells: [],
    timezone: "Europe/Moscow",
  });
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [reload, setReload] = useState(0);
  useEffect(() => {
    const key = `${week}:${identity}:${enabled}`;
    if (loaded.current !== key)
      setData({ cells: [], timezone: "Europe/Moscow" });
    setError("");
    if (!enabled) return;
    const c = new AbortController();
    let initial = true;
    async function load() {
      if (initial) setBusy(true);
      setError("");
      try {
        if (initial) {
          const cached = await api<WorklogWeek>(
            `/jira/worklogs?from=${week}&to=${shiftDay(week, 6)}`,
            "GET",
            undefined,
            c.signal,
          );
          if (c.signal.aborted) return;
          setData((old) => unchanged(old, cached));
          loaded.current = key;
        }
        const fresh = await api<WorklogWeek>(
          "/jira/worklogs/sync",
          "POST",
          { from: week, to: shiftDay(week, 6), force: initial && reload > 0 },
          AbortSignal.any([c.signal, AbortSignal.timeout(95000)]),
        );
        if (!c.signal.aborted) {
          setData((old) => unchanged(old, fresh));
          loaded.current = key;
          initial = false;
        }
      } catch (e) {
        if (!c.signal.aborted) setError((e as Error).message);
        initial = false;
        throw e;
      } finally {
        if (!c.signal.aborted) {
          setBusy(false);
        }
      }
    }
    poll(load, { signal: c.signal, interval: 15000 });
    return () => {
      c.abort();
    };
  }, [week, enabled, identity, reload]);
  return { data, error, busy, refresh: () => setReload((n) => n + 1) };
}
export function WorklogDay({
  member,
  date,
  onClose,
  onPlan,
}: {
  member: Member;
  date: string;
  onClose: () => void;
  onPlan: () => void;
}) {
  const [page, setPage] = useState(1),
    [data, setData] = useState<WorklogPage | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true);
  const [create, setCreate] = useState(false),
    [reload, setReload] = useState(0);
  useEffect(() => {
    const c = new AbortController();
    setLoading(true);
    setError("");
    poll(
      () =>
        api<WorklogPage>(
          `/jira/worklogs/day?member=${endpoint(member.id)}&date=${date}&page=${page}&limit=25`,
          "GET",
          undefined,
          c.signal,
        )
          .then((v) => {
            if (!c.signal.aborted) {
              setData((old) => unchanged(old, v));
              setError("");
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
  }, [member.id, date, page, reload]);
  if (create)
    return <JiraIssueEditor member={member} onClose={() => setCreate(false)} />;
  return (
    <Modal
      title={
        "Списания Jira · " + dayLabel(date, { day: "numeric", month: "long" })
      }
      wide
      onClose={onClose}
    >
      <div className="worklog-day">
        <strong>{member.name}</strong>
        <p className="muted">
          Фактически записанное время по автору списания. Даты — по Москве.
          Создание задачи не добавляет списанные часы.
        </p>
        <div className="jira-heading-actions">
          <button className="button small" onClick={() => setCreate(true)}>
            Создать задачу в Jira
          </button>
          <button className="button small" onClick={onPlan}>
            Планировать этот день
          </button>
        </div>
        {error && (
          <div role="alert" className="inline-warning">
            {error}
            <button
              className="button small"
              onClick={() => setReload((n) => n + 1)}
            >
              Повторить
            </button>
          </div>
        )}
        {loading ? (
          <p role="status">Загрузка списаний…</p>
        ) : data?.items.length ? (
          <div className="worklog-entries">
            {data.items.map((v) => (
              <article className="worklog-entry" key={v.id}>
                <div>
                  <a href={v.url} target="_blank" rel="noreferrer">
                    {v.issueKey} ↗
                  </a>
                  <strong>{v.title}</strong>
                  <small>
                    {new Date(v.started).toLocaleTimeString("ru-RU", {
                      timeZone: "Europe/Moscow",
                      hour: "2-digit",
                      minute: "2-digit",
                    })}{" "}
                    МСК
                  </small>
                  {v.comment && <p>{v.comment}</p>}
                </div>
                <b>{hoursText(v.seconds / 3600)} ч</b>
              </article>
            ))}
          </div>
        ) : (
          !error && (
            <Empty
              title="Списаний за этот день нет"
              description="Будущие часы появятся после записи времени в Jira. План на будущий день можно составить вручную."
            />
          )
        )}
        <TaskPagination data={data} onPage={setPage} disabled={loading} />
        <div className="modal-footer">
          <button className="button primary" onClick={onClose}>
            Закрыть
          </button>
        </div>
      </div>
    </Modal>
  );
}

import { useEffect, useRef, useState, type FormEvent } from "react";
import { poll, unchanged } from "../../shared/live";
import {
  ChevronLeft,
  ChevronRight,
  Plus,
  Settings2,
  Users,
  AlertTriangle,
  Download,
} from "lucide-react";
import { useData } from "../../app/store";
import {
  api,
  dayLabel,
  endpoint,
  shiftDay,
  weekStart,
  plural,
} from "../../shared/api";
import { Avatar, Empty, Modal } from "../../shared/ui";
import type { Member } from "../../shared/types";
import { DayPlan } from "./DayPlan";
import type { TeamWeekPlan } from "../../shared/types";
import { TeamSource } from "./TeamSource";
import { JiraIssueEditor } from "../integrations/JiraIssueEditor";
import { WorklogDay, useWorklogs, hoursText } from "./Worklogs";
import { WorklogExport } from "./WorklogExport";
import "./team.css";
export function Team() {
  const { state, act } = useData();
  const members = state.members.filter((m) => !m.inactive);
  const archived = state.members.filter((m) => m.inactive);
  const [week, setWeek] = useState(weekStart());
  const [member, setMember] = useState<Partial<Member> | null>(null);
  const [busy, setBusy] = useState(false);
  const days = Array.from({ length: 7 }, (_, i) => shiftDay(week, i));
  const [manualPlan, setPlan] = useState<TeamWeekPlan>({ cells: [] });
  const [actual, setActual] = useState(!!state.taskMetrics?.worklogAvailable);
  const [createJira, setCreateJira] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [worklogDay, setWorklogDay] = useState<{
    member: Member;
    date: string;
  } | null>(null);
  const worklogs = useWorklogs(
    week,
    actual && !state.settings.demo && state.settings.modules.jira,
    members
      .map((m) => [m.id, m.jiraSite, m.jiraUserId, m.jiraLogin].join(":"))
      .sort()
      .join("|"),
  );
  const plan: TeamWeekPlan = actual
    ? {
        cells: worklogs.data.cells.map((c) => ({
          memberId: c.memberId,
          date: c.date,
          hours: c.seconds / 3600,
          count: c.count,
          tasks: [],
          unestimated: 0,
        })),
      }
    : manualPlan;
  const [planError, setPlanError] = useState("");
  const [loading, setLoading] = useState(true);
  const loadedPlan = useRef("");
  const [reload, setReload] = useState(0);
  const [dayPlan, setDayPlan] = useState<{
    member: Member;
    date: string;
  } | null>(null);
  useEffect(() => {
    if (actual) {
      setLoading(false);
      setPlanError("");
      return;
    }
    const c = new AbortController();
    const key = `${week}:${state.settings.demo}`;
    setLoading(loadedPlan.current !== key);
    setPlanError("");
    if (loadedPlan.current !== key) setPlan({ cells: [] });
    poll(
      () =>
        api<TeamWeekPlan>(
          "/team/plan?from=" + week + "&to=" + shiftDay(week, 6),
          "GET",
          undefined,
          c.signal,
        )
          .then((v) => {
            if (!c.signal.aborted) {
              setPlan((old) => unchanged(old, v));
              loadedPlan.current = key;
              setPlanError("");
            }
          })
          .catch((e) => {
            if (!c.signal.aborted) setPlanError(e.message);
            throw e;
          })
          .finally(() => {
            if (!c.signal.aborted) setLoading(false);
          }),
      { signal: c.signal, interval: 10000 },
    );
    return () => c.abort();
  }, [week, state.revision, state.settings.demo, reload, actual]);
  const hours = (id: string) =>
    plan.cells
      .filter((c) => c.memberId === id)
      .reduce((n, c) => n + c.hours, 0);
  const overloaded = members.filter((m) => hours(m.id) > m.capacity);
  const formerCount = archived.reduce(
    (n, m) => n + (state.taskMetrics?.memberActive[m.id] || 0),
    0,
  );
  const unassignedCount = state.taskMetrics?.memberActive[""] || 0;
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!member) return;
    setBusy(true);
    try {
      await act(
        () =>
          api(
            "/members" + (member.id ? "/" + endpoint(member.id) : ""),
            member.id ? "PUT" : "POST",
            member,
          ),
        "Участник сохранён",
      );
      setMember(null);
    } catch {
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <div className="page-heading heading-flex">
        <div>
          <div className="eyebrow">ЛЮДИ И РЕСУРСЫ</div>
          <h1>Загрузка команды</h1>
          <p>Списанные часы из Jira и планы команды по дням.</p>
        </div>
        <div className="jira-heading-actions">
          <button
            className="button"
            disabled={
              state.settings.demo ||
              !state.settings.modules.jira ||
              !members.length
            }
            title={
              state.settings.demo
                ? "Выгрузка доступна в рабочем режиме"
                : "Выгрузить фактические списания Jira"
            }
            onClick={() => setExportOpen(true)}
          >
            <Download size={17} />
            Выгрузить часы
          </button>
          {state.settings.modules.jira && !state.settings.demo && (
            <button
              className="button primary"
              onClick={() => setCreateJira(true)}
            >
              <Plus size={17} />
              Создать в Jira
            </button>
          )}
          <button
            className="button"
            onClick={() =>
              setMember({ name: "", role: "Developer", capacity: 40 })
            }
          >
            <Plus size={17} />
            Участник
          </button>
        </div>
      </div>
      <TeamSource />
      {exportOpen && (
        <WorklogExport week={week} close={() => setExportOpen(false)} />
      )}
      <div className="team-stats">
        <div>
          <Users size={20} />
          <span>
            <strong>{members.length}</strong> участников
          </span>
        </div>
        <div>
          <strong>
            {actual && !worklogs.data.syncedAt
              ? "—"
              : hoursText(members.reduce((n, m) => n + hours(m.id), 0)) + " ч"}
          </strong>
          <span>{actual ? "списано в Jira" : "запланировано"}</span>
        </div>
        <div>
          <strong>{members.reduce((n, m) => n + m.capacity, 0)} ч</strong>
          <span>доступно в неделю</span>
        </div>
        <div className={overloaded.length ? "overdue" : ""}>
          <strong>{overloaded.length}</strong>
          <span>с перегрузкой</span>
        </div>
      </div>
      <div className="planner-toolbar">
        <div className="segmented" aria-label="Источник загрузки">
          <button
            className={actual ? "active" : ""}
            disabled={!state.taskMetrics?.worklogAvailable}
            onClick={() => setActual(true)}
          >
            Факт Jira
          </button>
          <button
            className={!actual ? "active" : ""}
            onClick={() => setActual(false)}
          >
            План
          </button>
        </div>
        <div className="period-switch">
          <button
            className="icon-button"
            aria-label="Предыдущая неделя"
            onClick={() => setWeek(shiftDay(week, -7))}
          >
            <ChevronLeft size={18} />
          </button>
          <strong>
            {dayLabel(week)} — {dayLabel(days[6])}
          </strong>
          <button
            className="icon-button"
            aria-label="Следующая неделя"
            onClick={() => setWeek(shiftDay(week, 7))}
          >
            <ChevronRight size={18} />
          </button>
          <button className="button small" onClick={() => setWeek(weekStart())}>
            Эта неделя
          </button>
        </div>
        <span className="muted">
          {actual
            ? "Нажмите на день, чтобы увидеть списания"
            : "Нажмите на день сотрудника, чтобы выбрать несколько задач"}
        </span>
      </div>
      {actual && (
        <div className="worklog-sync">
          <span>
            {worklogs.busy
              ? "Обновление списаний Jira…"
              : worklogs.data.syncedAt
                ? "Обновлено " +
                  new Date(worklogs.data.syncedAt).toLocaleString("ru-RU", {
                    timeZone: "Europe/Moscow",
                  }) +
                  " МСК"
                : "Часы за эту неделю ещё не загружены"}
          </span>
          <button
            className="button small"
            disabled={worklogs.busy}
            onClick={worklogs.refresh}
          >
            Обновить часы
          </button>
        </div>
      )}
      {actual && worklogs.error && (
        <div className="inline-warning" role="alert">
          {worklogs.error}
        </div>
      )}
      {actual && worklogs.data.warning && (
        <div className="info-strip">{worklogs.data.warning}</div>
      )}
      {planError && (
        <div className="inline-warning" role="alert">
          {planError}
          <button
            className="button small"
            onClick={() => setReload((n) => n + 1)}
          >
            Повторить
          </button>
        </div>
      )}
      {loading && (
        <p className="muted" role="status">
          Загрузка плана…
        </p>
      )}
      {members.length ? (
        <div className="panel table-wrap capacity-table">
          <table>
            <thead>
              <tr>
                <th>Участник / загрузка</th>
                {days.map((d) => (
                  <th key={d}>
                    {dayLabel(d, { weekday: "short" })}
                    <span>
                      {dayLabel(d, { day: "numeric", month: "short" })}
                    </span>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {members.map((m) => {
                const h = hours(m.id);
                return (
                  <tr key={m.id}>
                    <td>
                      <div className="capacity-person">
                        <Avatar member={m} />
                        <div>
                          <strong>{m.name}</strong>
                          <small>{m.role || "Должность не указана"}</small>
                          {m.departments?.length ? (
                            <small className="team-member-departments">
                              {m.departments.map((d) => d.name).join(" · ")}
                            </small>
                          ) : null}
                          {m.source === "bitrix" && (
                            <span className="team-member-badge">Битрикс24</span>
                          )}
                        </div>
                        <button
                          className="icon-button"
                          aria-label={"Настроить " + m.name}
                          onClick={() => setMember(m)}
                        >
                          <Settings2 size={15} />
                        </button>
                      </div>
                      <div className="capacity-meta">
                        <span className={h > m.capacity ? "overdue" : ""}>
                          {actual && !worklogs.data.syncedAt
                            ? "—"
                            : hoursText(h)}{" "}
                          / {m.capacity} ч
                        </span>
                        <span>
                          {m.capacity
                            ? Math.round((h / m.capacity) * 100) + "%"
                            : h
                              ? "Нет доступности"
                              : "0%"}
                        </span>
                      </div>
                      <div className="progress-line">
                        <span
                          className={h > m.capacity ? "too-much" : ""}
                          style={{
                            width:
                              Math.min(
                                100,
                                m.capacity
                                  ? (h / m.capacity) * 100
                                  : h
                                    ? 100
                                    : 0,
                              ) + "%",
                          }}
                        />
                      </div>
                    </td>
                    {days.map((d) => {
                      const cell = plan.cells.find(
                        (c) => c.memberId === m.id && c.date === d,
                      );
                      const actualCell = actual
                        ? worklogs.data.cells.find(
                            (c) => c.memberId === m.id && c.date === d,
                          )
                        : undefined;
                      return (
                        <td key={d} className="team-day-cell">
                          <button
                            className={
                              "team-day-button " +
                              (cell?.count ? "has-plan" : "")
                            }
                            disabled={
                              loading ||
                              !!planError ||
                              (!actual && !state.settings.modules.backlog)
                            }
                            aria-label={
                              (actual ? "Списания: " : "План: ") +
                              m.name +
                              ", " +
                              dayLabel(d)
                            }
                            onClick={() =>
                              actual
                                ? setWorklogDay({ member: m, date: d })
                                : setDayPlan({ member: m, date: d })
                            }
                          >
                            {cell?.count ? (
                              <>
                                <span className="team-day-total">
                                  {actual
                                    ? hoursText(cell.hours) + " ч"
                                    : cell.count + " задач"}
                                  {actual
                                    ? " · " +
                                      cell.count +
                                      " " +
                                      plural(
                                        cell.count,
                                        "списание",
                                        "списания",
                                        "списаний",
                                      )
                                    : cell.hours
                                      ? " · " + hoursText(cell.hours) + " ч"
                                      : ""}
                                </span>
                                {actualCell && (
                                  <span className="team-worklog-tasks">
                                    {actualCell.tasks?.map((task) => (
                                      <span
                                        className="team-worklog-task"
                                        key={task.issueKey}
                                      >
                                        <span className="team-worklog-key">
                                          {task.issueKey}
                                        </span>
                                        <span
                                          className="team-worklog-title"
                                          title={task.title}
                                        >
                                          {task.title || "Без названия"}
                                        </span>
                                        <strong className="team-worklog-hours">
                                          {hoursText(task.seconds / 3600)} ч
                                        </strong>
                                      </span>
                                    ))}
                                    {actualCell.taskCount >
                                      (actualCell.tasks?.length ?? 0) && (
                                      <span className="team-worklog-more">
                                        Ещё{" "}
                                        {actualCell.taskCount -
                                          (actualCell.tasks?.length ?? 0)}{" "}
                                        задач · открыть день
                                      </span>
                                    )}
                                  </span>
                                )}
                                {cell.tasks.map((t) => (
                                  <span className="allocation" key={t.id}>
                                    <strong>{t.title}</strong>
                                    <small>{t.externalId || t.project}</small>
                                  </span>
                                ))}
                                {!actual && cell.count > 3 && (
                                  <span className="muted">
                                    Ещё {cell.count - 3}
                                  </span>
                                )}
                                {cell.unestimated > 0 && (
                                  <small className="muted">
                                    Без часов: {cell.unestimated}
                                  </small>
                                )}
                              </>
                            ) : (
                              <span className="team-day-empty">
                                {!actual && <Plus size={16} />}
                                {actual
                                  ? worklogs.data.syncedAt && m.jiraUserId
                                    ? "Нет списаний"
                                    : "Нет данных"
                                  : "Выбрать задачи"}
                              </span>
                            )}
                          </button>
                        </td>
                      );
                    })}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <Empty
          title="Соберите свою команду"
          description="Добавьте участников и их доступные часы в неделю."
        />
      )}
      {overloaded.length > 0 && (
        <div className="inline-warning margin-top">
          <AlertTriangle size={17} />У {overloaded.length} участников{" "}
          {actual ? "списанное время превышает" : "план превышает"} доступность
          на неделю.
        </div>
      )}
      <div className="info-strip margin-top">
        {actual
          ? "Факт — списания Jira по автору и дате начала работы (Москва), включая закрытые задачи и задачи других исполнителей. Обновляется каждую минуту. Будущие планы из списаний определить нельзя — для них доступен режим «План»."
          : "План — задачи и часы, выбранные вручную на каждый день. Можно выбрать несколько задач и повторять задачу в разные дни. План и списанные часы Jira считаются отдельно."}
      </div>
      {(formerCount > 0 || unassignedCount > 0) && (
        <div className="info-strip margin-top">
          Без исполнителя: {unassignedCount}. У сотрудников вне активного
          состава: {formerCount}. <a href="#backlog">Проверить в бэклоге</a>
        </div>
      )}
      {archived.length > 0 && (
        <details className="team-archive">
          <summary>Вне активного состава · {archived.length}</summary>
          <p>
            Эти сотрудники больше не входят в вашу структуру. Их прежние задачи
            и история сохранены.
          </p>
          <ul>
            {archived.map((m) => (
              <li key={m.id}>
                <strong>{m.name}</strong>
                <small>
                  {m.role} · {state.taskMetrics?.memberTotals[m.id] || 0} задач
                  в истории
                </small>
              </li>
            ))}
          </ul>
        </details>
      )}
      {dayPlan && (
        <DayPlan
          key={dayPlan.member.id + dayPlan.date}
          {...dayPlan}
          onClose={() => setDayPlan(null)}
        />
      )}
      {createJira && <JiraIssueEditor onClose={() => setCreateJira(false)} />}
      {worklogDay && (
        <WorklogDay
          key={worklogDay.member.id + worklogDay.date}
          {...worklogDay}
          onClose={() => setWorklogDay(null)}
          onPlan={() => {
            setDayPlan(worklogDay);
            setWorklogDay(null);
          }}
        />
      )}
      {member && (
        <Modal
          title={member.id ? "Доступность участника" : "Новый участник"}
          onClose={() => setMember(null)}
        >
          <form className="form" onSubmit={save}>
            <label>
              Имя
              <input
                required
                maxLength={120}
                readOnly={member.source === "bitrix"}
                value={member.name}
                onChange={(e) => setMember({ ...member, name: e.target.value })}
              />
            </label>
            <label>
              Роль
              <input
                readOnly={member.source === "bitrix"}
                value={member.role}
                onChange={(e) => setMember({ ...member, role: e.target.value })}
              />
            </label>
            <label>
              Доступно для задач, часов в неделю
              <input
                type="number"
                min={0}
                max={80}
                step="0.5"
                required
                value={member.capacity}
                onChange={(e) =>
                  setMember({ ...member, capacity: Number(e.target.value) })
                }
              />
            </label>
            {member.source === "bitrix" && (
              <p className="muted">
                Имя, должность и отдел обновляются из Битрикса. Здесь можно
                изменить доступность для задач.
              </p>
            )}
            <p className="muted">
              Вычтите встречи, поддержку и другие регулярные обязанности из
              рабочего времени.
            </p>
            <div className="modal-footer">
              <button
                className="button"
                type="button"
                onClick={() => setMember(null)}
              >
                Отмена
              </button>
              <button className="button primary" disabled={busy}>
                Сохранить
              </button>
            </div>
          </form>
        </Modal>
      )}
    </>
  );
}

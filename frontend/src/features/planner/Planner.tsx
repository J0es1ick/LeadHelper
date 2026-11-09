import { useState } from "react";
import {
  Plus,
  ChevronLeft,
  ChevronRight,
  CalendarDays,
  CheckCheck,
} from "lucide-react";
import { useData } from "../../app/store";
import {
  api,
  dateKey,
  dayLabel,
  endpoint,
  shiftDay,
  weekStart,
} from "../../shared/api";
import { TaskMini, Empty } from "../../shared/ui";
import type { Task } from "../../shared/types";
import { BitrixCalendar, DayEvent, useBitrixCalendar } from "./BitrixCalendar";
import {
  useTaskPage,
  TaskPagination,
  TaskPageFeedback,
} from "../../shared/taskPages";
import { eventsForDay } from "./calendar";
export function Planner({
  edit,
}: {
  edit: (t?: Task, personal?: boolean, date?: string) => void;
}) {
  const { state, act } = useData();
  const [day, setDay] = useState(dateKey());
  const [view, setView] = useState("week");
  const start = weekStart(day);
  const selectedPage = useTaskPage({
    personal: "true",
    from: day,
    to: day,
    limit: 25,
  });
  const donePage = useTaskPage({
    personal: "true",
    from: day,
    to: day,
    status: "done",
    limit: 1,
  });
  const undatedPage = useTaskPage({
    personal: "true",
    from: "none",
    limit: 10,
  });
  const days = Array.from({ length: 7 }, (_, i) => shiftDay(start, i));
  const calendar = useBitrixCalendar(start, days[6]);
  const selected = selectedPage.data?.items || [];
  const total = selectedPage.data?.total || 0;
  const done = donePage.data?.total || 0;
  const toggle = (t: Task) =>
    void act(() =>
      api("/tasks/" + endpoint(t.id) + "/status", "PATCH", {
        status: t.status === "done" ? "todo" : "done",
      }),
    ).catch(() => {});
  return (
    <>
      <div className="page-heading heading-flex">
        <div>
          <div className="eyebrow">ЛИЧНОЕ ПРОСТРАНСТВО</div>
          <h1>Мой план</h1>
          <p>Освободите голову. Оставьте место для важного.</p>
        </div>
        <button
          className="button primary"
          onClick={() => edit(undefined, true, day)}
        >
          <Plus size={17} />
          Добавить задачу
        </button>
      </div>
      <div className="planner-toolbar">
        <div className="period-switch">
          <button
            className="icon-button"
            aria-label="Предыдущий период"
            onClick={() => setDay(shiftDay(day, view === "week" ? -7 : -1))}
          >
            <ChevronLeft size={18} />
          </button>
          <strong>
            {view === "week"
              ? `${dayLabel(start)} — ${dayLabel(days[6])}`
              : dayLabel(day, {
                  day: "numeric",
                  month: "long",
                  weekday: "long",
                })}
          </strong>
          <button
            className="icon-button"
            aria-label="Следующий период"
            onClick={() => setDay(shiftDay(day, view === "week" ? 7 : 1))}
          >
            <ChevronRight size={18} />
          </button>
          <button className="button small" onClick={() => setDay(dateKey())}>
            Сегодня
          </button>
        </div>
        <div className="segmented">
          <button
            className={view === "day" ? "active" : ""}
            onClick={() => setView("day")}
          >
            День
          </button>
          <button
            className={view === "week" ? "active" : ""}
            onClick={() => setView("week")}
          >
            Неделя
          </button>
        </div>
      </div>
      {view === "week" && (
        <div className="week-grid">
          {days.map((d) => (
            <section
              key={d}
              className={
                "day-column " +
                (d === dateKey() ? "is-today" : "") +
                " " +
                (d === day ? "selected" : "")
              }
            >
              <button className="day-heading" onClick={() => setDay(d)}>
                <span>{dayLabel(d, { weekday: "short" })}</span>
                <strong>{dayLabel(d, { day: "numeric" })}</strong>
                {d === dateKey() && <i />}
              </button>
              <div className="day-tasks">
                {eventsForDay(calendar.data?.events || [], d).map((e) => (
                  <DayEvent event={e} compact key={e.id} />
                ))}
                <PersonalDayPreview
                  day={d}
                  edit={edit}
                  select={() => setDay(d)}
                />
              </div>
              <button
                className="add-day"
                aria-label={"Добавить задачу " + dayLabel(d)}
                onClick={() => edit(undefined, true, d)}
              >
                <Plus size={16} />
              </button>
            </section>
          ))}
        </div>
      )}
      <BitrixCalendar day={day} calendar={calendar} />
      <div className="planner-detail">
        <section className="panel">
          <div className="section-heading">
            <div>
              <h2>
                {dayLabel(day, {
                  weekday: "long",
                  day: "numeric",
                  month: "long",
                })}
              </h2>
              <p>{total} задач</p>
            </div>
            <span className="small-icon mint">
              <CalendarDays size={20} />
            </span>
          </div>
          <TaskPageFeedback {...selectedPage} />
          {selected.length ? (
            selected.map((t) => (
              <TaskMini
                key={t.id}
                task={t}
                members={state.members}
                onClick={() => edit(t)}
                onToggle={() => toggle(t)}
              />
            ))
          ) : !selectedPage.loading && !selectedPage.error ? (
            <Empty
              title="Личных задач пока нет"
              description="Добавьте одну главную задачу, с которой хотите начать."
              action={
                <button
                  className="button"
                  onClick={() => edit(undefined, true, day)}
                >
                  <Plus size={16} />
                  Запланировать
                </button>
              }
            />
          ) : null}
          <TaskPagination
            data={selectedPage.data}
            onPage={selectedPage.setPage}
          />
          <button
            className="add-inline"
            onClick={() => edit(undefined, true, day)}
          >
            <Plus size={17} />
            Добавить в план
          </button>
        </section>
        <aside className="plan-summary">
          <CheckCheck size={25} />
          <h3>
            {done} из {total}
          </h3>
          <p>задач выполнено</p>
          <div className="progress-line">
            <span
              style={{
                width: (total ? (done / total) * 100 : 0) + "%",
              }}
            />
          </div>
          <div className="plan-tip">
            Хороший план оставляет место для неожиданного. Не заполняйте
            задачами весь рабочий день.
          </div>
        </aside>
      </div>
      <section className="panel">
        <div className="section-heading">
          <h2>Без даты · {undatedPage.data?.total ?? "…"}</h2>
        </div>
        <TaskPageFeedback {...undatedPage} />
        {undatedPage.data?.items.map((t) => (
          <TaskMini
            key={t.id}
            task={t}
            members={state.members}
            onClick={() => edit(t)}
            onToggle={() => toggle(t)}
          />
        ))}
        <TaskPagination data={undatedPage.data} onPage={undatedPage.setPage} />
      </section>
    </>
  );
}

function PersonalDayPreview({
  day,
  edit,
  select,
}: {
  day: string;
  edit: (t: Task) => void;
  select: () => void;
}) {
  const page = useTaskPage({ personal: "true", from: day, to: day, limit: 5 });
  return (
    <>
      <TaskPageFeedback {...page} />
      {page.data?.items.map((t) => (
        <button
          className={"day-task " + (t.status === "done" ? "completed" : "")}
          key={t.id}
          onClick={() => edit(t)}
        >
          <span className={"status-dot " + t.status} />
          {t.title}
          <small>{t.estimate} ч</small>
        </button>
      ))}
      {(page.data?.total || 0) > 5 && (
        <button className="text-button" onClick={select}>
          Все задачи: {page.data?.total}
        </button>
      )}
    </>
  );
}

import {
  ArrowUpRight,
  CheckCheck,
  Clock3,
  Flag,
  Mail,
  Plus,
  ArrowRight,
  Sun,
  CalendarDays,
  AlertCircle,
  StickyNote,
} from "lucide-react";
import { useData } from "../../app/store";
import {
  api,
  dateKey,
  dayLabel,
  endpoint,
  weekStart,
  shiftDay,
  plural,
} from "../../shared/api";
import {
  Avatar,
  Empty,
  SectionHead,
  Source,
  TaskMini,
  ViewLink,
} from "../../shared/ui";
import type { Page, Task } from "../../shared/types";
export function Dashboard({
  navigate,
  edit,
}: {
  navigate: (p: Page) => void;
  edit: (t?: Task, personal?: boolean) => void;
}) {
  const { state, act } = useData();
  const { tasks, members, messages, settings, notes } = state;
  const currentMembers = members.filter((m) => !m.inactive);
  const today = dateKey();
  const team = tasks.filter((t) => !t.personal);
  const active = team.filter((t) => t.status !== "done");
  const metrics = state.taskMetrics;
  const doneCount = metrics?.statuses.done || 0;
  const activeCount = Object.entries(metrics?.statuses || {}).reduce(
    (n, [s, count]) => (s === "done" ? n : n + count),
    0,
  );
  const overdueCount = metrics?.overdue || 0;
  const personalTotal = metrics?.personalTotal || 0,
    personalDone = metrics?.personalDone || 0;
  const personalActive = personalTotal - personalDone;
  const personal = tasks.filter((t) => t.personal && t.plannedDate === today);
  const unread =
    state.mailStats?.unread ??
    messages.filter((m) => m.folder === "inbox" && !m.read).length;
  const start = weekStart();
  const end = shiftDay(start, 6);
  const actual = !!metrics?.worklogAvailable;
  const actualMissing = actual && !metrics?.worklogSyncedAt;
  const workloads = currentMembers.map((m) => ({
    member: m,
    hours:
      (actual ? metrics?.worklogHours?.[m.id] : metrics?.memberHours[m.id]) ||
      0,
  }));
  const totalCapacity = currentMembers.reduce((n, m) => n + m.capacity, 0);
  const hours = workloads.reduce((n, w) => n + w.hours, 0);
  const percent = totalCapacity ? Math.round((hours / totalCapacity) * 100) : 0;
  const toggle = (t: Task) =>
    void act(() =>
      api("/tasks/" + endpoint(t.id) + "/status", "PATCH", {
        status: t.status === "done" ? "todo" : "done",
      }),
    ).catch(() => {});
  return (
    <>
      <div className="page-heading">
        <div className="eyebrow">ВАШ РАБОЧИЙ ОБЗОР</div>
        <h1>
          Всё важное. В одном месте<span className="green">.</span>
        </h1>
        <p>Команда, планы и рабочий контекст — под рукой.</p>
      </div>
      <div className="dashboard-grid">
        <section className="welcome-card">
          <div>
            <div className="welcome-kicker">
              <Sun size={16} />
              {dayLabel(today, {
                weekday: "long",
                day: "numeric",
                month: "long",
              })}
            </div>
            <h2>Хорошего дня, {settings.name.split(" ")[0]}.</h2>
            <p>
              {personalActive ? (
                <>
                  В личном плане —{" "}
                  <strong>
                    {personalActive}{" "}
                    {plural(personalActive, "задача", "задачи", "задач")}
                  </strong>
                  .<br />
                  Начните с главного, остальное уже собрано здесь.
                </>
              ) : (
                <>
                  Есть время расставить приоритеты.
                  <br />
                  Добавьте важные задачи в план на сегодня.
                </>
              )}
            </p>
            {settings.modules.planner && (
              <button
                className="button hero-button"
                onClick={() => navigate("planner")}
              >
                Мой план на день
                <ArrowRight size={16} />
              </button>
            )}
          </div>
          <div className="hero-art" aria-hidden="true">
            <div className="orbit orbit-one" />
            <div className="orbit orbit-two" />
            <div className="hero-tile tile-back">
              <CheckCheck size={30} />
            </div>
            <div className="hero-tile tile-front">
              <span className="tiny-line" />
              <span className="tile-check">✓</span>
              <span className="tiny-line short" />
              <span className="tiny-line" />
            </div>
            <div className="hero-dot" />
          </div>
        </section>
        <section className="focus-card">
          <div className="card-label">
            <span className="small-icon amber">
              <Flag size={17} />
            </span>
            Требует внимания
          </div>
          <h2>
            {overdueCount
              ? `Просрочено задач: ${overdueCount}`
              : "Все сроки под контролем"}
          </h2>
          <p>
            {overdueCount
              ? "Проверьте блокеры и договоритесь с командой о новых сроках."
              : "Сейчас нет просроченных задач команды. Можно сосредоточиться на плане."}
          </p>
          {settings.modules.backlog && (
            <button className="text-button" onClick={() => navigate("backlog")}>
              Перейти в бэклог
              <ArrowUpRight size={16} />
            </button>
          )}
        </section>
      </div>
      <div className="stats-grid">
        {[
          {
            label: "Задач в работе",
            value:
              (metrics?.statuses.progress || 0) +
              (metrics?.statuses.review || 0),
            detail: `${activeCount} открытых задач`,
            icon: Clock3,
            color: "mint",
            page: "backlog",
            module: "backlog",
          },
          {
            label: "Завершено задач",
            value: doneCount,
            detail: "В текущем пространстве",
            icon: CheckCheck,
            color: "purple",
            page: "backlog",
            module: "backlog",
          },
          {
            label: "Загрузка команды",
            value: actualMissing ? "—" : percent + "%",
            detail: actualMissing
              ? "Списания Jira ещё не загружены"
              : `${actual ? "Факт Jira: " : "План: "}${hours.toLocaleString("ru-RU", { maximumFractionDigits: 2 })} из ${totalCapacity} ч на неделю`,
            icon: CalendarDays,
            color: "blue",
            page: "team",
            module: "team",
          },
          {
            label: "Непрочитанных писем",
            value: unread,
            detail: "Во входящих",
            icon: Mail,
            color: "amber",
            page: "mail",
            module: "mail",
          },
        ]
          .filter((x) => settings.modules[x.module])
          .map((item) => (
            <button
              className="stat-card"
              key={item.label}
              onClick={() => navigate(item.page as Page)}
            >
              <div className="stat-top">
                <span>{item.label}</span>
                <span className={"small-icon " + item.color}>
                  <item.icon size={18} />
                </span>
              </div>
              <strong>{item.value}</strong>
              <div className="stat-bottom">
                {item.detail}
                <ArrowUpRight size={15} />
              </div>
            </button>
          ))}
      </div>
      <div className="dashboard-columns">
        <div className="dashboard-main">
          {settings.modules.backlog && (
            <section className="panel">
              <SectionHead
                title="В фокусе команды"
                subtitle="Высокие приоритеты и ближайшие сроки"
                action={
                  <ViewLink onClick={() => navigate("backlog")}>
                    Все задачи
                  </ViewLink>
                }
              />
              <div className="compact-table-head">
                <span>ЗАДАЧА</span>
                <span>СРОК / ИСПОЛНИТЕЛЬ</span>
              </div>
              {active.length ? (
                active
                  .slice()
                  .sort((a, b) =>
                    (a.dueDate || "9999").localeCompare(b.dueDate || "9999"),
                  )
                  .slice(0, 5)
                  .map((t) => (
                    <TaskMini
                      key={t.id}
                      task={t}
                      members={members}
                      onClick={() => edit(t)}
                    />
                  ))
              ) : (
                <Empty
                  title="Пока нет задач"
                  action={
                    <button className="button" onClick={() => edit()}>
                      Создать первую
                    </button>
                  }
                />
              )}
              <div className="panel-foot">
                <span className="live-dot" />
                Данные из подключённых модулей
                {settings.demo && (
                  <span className="subtle"> · демонстрация</span>
                )}
              </div>
            </section>
          )}
          {settings.modules.team && (
            <section className="panel">
              <SectionHead
                title="Пульс команды"
                subtitle={`${actual ? "Факт Jira" : "План"} на ${dayLabel(start)} — ${dayLabel(end)}`}
                action={
                  <ViewLink onClick={() => navigate("team")}>
                    План загрузки
                  </ViewLink>
                }
              />
              <div className="workload-list">
                {workloads.slice(0, 5).map(({ member: m, hours: h }) => (
                  <div className="workload-row" key={m.id}>
                    <Avatar member={m} />
                    <div className="member-info">
                      <strong>{m.name}</strong>
                      <span>{m.role}</span>
                    </div>
                    <div className="workload-track">
                      <div
                        style={{
                          width:
                            Math.min(
                              100,
                              m.capacity ? (h / m.capacity) * 100 : h ? 100 : 0,
                            ) + "%",
                        }}
                        className={h > m.capacity ? "too-much" : ""}
                      />
                    </div>
                    <span
                      className={"hours " + (h > m.capacity ? "overdue" : "")}
                    >
                      {actualMissing
                        ? "—"
                        : h.toLocaleString("ru-RU", {
                            maximumFractionDigits: 2,
                          })}
                      <span> / {m.capacity} ч</span>
                    </span>
                  </div>
                ))}
              </div>
              {!currentMembers.length && (
                <Empty title="Добавьте участников команды" />
              )}
              {workloads.some((w) => w.hours > w.member.capacity) && (
                <div className="inline-warning">
                  <AlertCircle size={15} />
                  Есть перегрузка: перераспределите задачи до начала недели.
                </div>
              )}
            </section>
          )}
        </div>
        <div className="dashboard-aside">
          {settings.modules.planner && (
            <section className="panel today-panel">
              <SectionHead
                title="Мой день"
                subtitle={dayLabel(today, { day: "numeric", month: "long" })}
                action={
                  <span className="count-badge">
                    {personalDone}/{personalTotal}
                  </span>
                }
              />
              <div className="progress-line">
                <span
                  style={{
                    width:
                      (personalTotal
                        ? (personalDone / personalTotal) * 100
                        : 0) + "%",
                  }}
                />
              </div>
              {personal.length ? (
                personal.map((t) => (
                  <div className="personal-row" key={t.id}>
                    <button
                      className={
                        "check " + (t.status === "done" ? "checked" : "")
                      }
                      onClick={() => toggle(t)}
                      aria-label={"Завершить: " + t.title}
                    >
                      {t.status === "done" ? "✓" : ""}
                    </button>
                    <button
                      className={t.status === "done" ? "completed" : ""}
                      onClick={() => edit(t)}
                    >
                      {t.title}
                    </button>
                  </div>
                ))
              ) : (
                <p className="muted pad">План на сегодня пока свободен.</p>
              )}
              <button
                className="add-inline"
                onClick={() => navigate("planner")}
              >
                Весь план · {personalTotal} задач
              </button>
              <button
                className="add-inline"
                onClick={() => edit(undefined, true)}
              >
                <Plus size={17} />
                Добавить задачу
              </button>
            </section>
          )}
          {settings.modules.mail && (
            <section className="panel">
              <SectionHead
                title="Последние письма"
                action={
                  <button
                    className="icon-button"
                    onClick={() => navigate("mail")}
                    aria-label="Открыть почту"
                  >
                    <ArrowUpRight size={18} />
                  </button>
                }
              />
              {messages.slice(0, 3).map((m) => (
                <button
                  className="mail-preview"
                  key={m.id}
                  onClick={() => navigate("mail")}
                >
                  <Avatar name={m.fromName} />
                  <span>
                    <strong>{m.fromName}</strong>
                    <span>{m.subject}</span>
                  </span>
                  {!m.read && <i className="unread-dot" />}
                </button>
              ))}
              {!messages.length && (
                <p className="muted pad">Почтовый ящик пока пуст.</p>
              )}
            </section>
          )}
          {settings.modules.notes && (
            <section className="note-preview">
              <StickyNote size={19} />
              <span className="eyebrow">НА ЗАМЕТКУ</span>
              <h3>
                {notes.find((n) => n.pinned)?.title || "Место для ваших идей"}
              </h3>
              <p>
                {notes.find((n) => n.pinned)?.body.split("\n")[0] ||
                  "Сохраняйте договорённости, мысли и контекст встреч."}
              </p>
              <ViewLink onClick={() => navigate("notes")}>
                Открыть заметки
              </ViewLink>
            </section>
          )}
        </div>
      </div>
      <div className="source-footer">
        <span>Одно пространство для спокойной работы.</span>
        <div>
          {settings.modules.jira && <Source value="jira" />}
          {settings.modules.bitrix && <Source value="bitrix" />}
          {settings.modules.mail && (
            <span className="source">
              <Mail size={12} />
              Почта
            </span>
          )}
        </div>
      </div>
    </>
  );
}

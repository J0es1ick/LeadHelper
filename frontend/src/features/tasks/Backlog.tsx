import { useState } from "react";
import { Plus, Search, LayoutGrid, List, ArrowDownUp } from "lucide-react";
import { useData } from "../../app/store";
import { api, dateKey, dayLabel, endpoint } from "../../shared/api";
import { Avatar, Empty, PriorityBadge, Source } from "../../shared/ui";
import { statuses, type Status, type Task } from "../../shared/types";
import { JiraIssueEditor } from "../integrations/JiraIssueEditor";
import {
  TaskPagination,
  TaskPageFeedback,
  useTaskPage,
  useDebounced,
  type TaskFilters,
} from "../../shared/taskPages";
const columns = Object.keys(statuses) as Status[];
type Actions = {
  edit: (t?: Task) => void;
  move: (id: string, status: Status) => Promise<void>;
  busy: boolean;
};
export function Backlog({ edit }: { edit: (t?: Task) => void }) {
  const { state, act } = useData();
  const [createJira, setCreateJira] = useState(false);
  const [view, setView] = useState("board"),
    [search, setSearch] = useState("");
  const [source, setSource] = useState("all"),
    [assignee, setAssignee] = useState("all"),
    [priority, setPriority] = useState("all");
  const [sort, setSort] = useState(false),
    [busy, setBusy] = useState(false);
  const filters: TaskFilters = {
    personal: "false",
    source,
    assignee,
    priority,
    search: useDebounced(search),
    sort: sort ? "due" : "updated",
  };
  async function move(id: string, status: Status) {
    if (!id || busy) return;
    setBusy(true);
    try {
      await act(
        () => api(`/tasks/${endpoint(id)}/status`, "PATCH", { status }),
        "Статус обновлён",
      );
    } catch {
    } finally {
      setBusy(false);
    }
  }
  const actions = { edit, move, busy };
  return (
    <>
      <div className="page-heading heading-flex">
        <div>
          <div className="eyebrow">КОМАНДНАЯ РАБОТА</div>
          <h1>Бэклог и задачи</h1>
          <p>От идеи до результата. Все источники — на одной доске.</p>
        </div>
        <div className="jira-heading-actions">
          <button className="button" onClick={() => edit()}>
            <Plus size={17} />
            Локальная задача
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
        </div>
      </div>
      {createJira && <JiraIssueEditor onClose={() => setCreateJira(false)} />}
      <div className="toolbar">
        <div className="search-field">
          <Search size={17} />
          <input
            aria-label="Поиск задач"
            placeholder="Поиск по задачам…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <select
          aria-label="Источник задач"
          value={source}
          onChange={(e) => setSource(e.target.value)}
        >
          <option value="all">Все источники</option>
          <option value="local">LeadHelper</option>
          {state.settings.modules.jira && <option value="jira">Jira</option>}
        </select>
        <select
          aria-label="Исполнитель"
          value={assignee}
          onChange={(e) => setAssignee(e.target.value)}
        >
          <option value="all">Все исполнители</option>
          <option value="">Без исполнителя</option>
          {state.members.map((m) => (
            <option key={m.id} value={m.id}>
              {m.name}
              {m.inactive ? " · вне команды" : ""}
            </option>
          ))}
        </select>
        <select
          aria-label="Приоритет"
          value={priority}
          onChange={(e) => setPriority(e.target.value)}
        >
          <option value="all">Все приоритеты</option>
          <option value="critical">Критический</option>
          <option value="high">Высокий</option>
          <option value="medium">Средний</option>
          <option value="low">Низкий</option>
        </select>
        <button
          title="Сортировать по сроку"
          aria-pressed={sort}
          className={"icon-button " + (sort ? "active" : "")}
          onClick={() => setSort(!sort)}
        >
          <ArrowDownUp size={18} />
        </button>
        <div className="segmented">
          <button
            className={view === "board" ? "active" : ""}
            onClick={() => setView("board")}
            aria-label="Доска"
          >
            <LayoutGrid size={17} />
          </button>
          <button
            className={view === "list" ? "active" : ""}
            onClick={() => setView("list")}
            aria-label="Список"
          >
            <List size={17} />
          </button>
        </div>
      </div>
      <div className="view-caption">
        <span>
          {view === "board"
            ? "По 20 задач на страницу в каждой колонке"
            : "По 25 задач на страницу"}
        </span>
        <span>
          {view === "board"
            ? "Перетаскивайте карточки между колонками"
            : "Статусы можно менять прямо в таблице"}
        </span>
      </div>
      {view === "board" ? (
        <div className="kanban">
          {columns.map((status) => (
            <TaskColumn
              key={status}
              status={status}
              filters={filters}
              {...actions}
            />
          ))}
        </div>
      ) : (
        <TaskList filters={filters} {...actions} />
      )}
    </>
  );
}
function TaskColumn({
  status,
  filters,
  edit,
  move,
  busy,
}: Actions & { status: Status; filters: TaskFilters }) {
  const { state } = useData();
  const page = useTaskPage({ ...filters, status, limit: 20 });
  const [dragging, setDragging] = useState("");
  return (
    <section
      className="kanban-column"
      aria-label={statuses[status]}
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault();
        void move(e.dataTransfer.getData("text/plain"), status);
      }}
    >
      <div className="column-heading">
        <span className={"status-dot " + status} />
        <h3>{statuses[status]}</h3>
        <span>{page.data?.total ?? "…"}</span>
      </div>
      <div className="column-cards">
        <TaskPageFeedback {...page} />
        {page.data?.items.map((t) => (
          <article
            className={"task-card " + (dragging === t.id ? "dragging" : "")}
            key={t.id}
            draggable={!busy}
            onDragStart={(e) => {
              e.dataTransfer.setData("text/plain", t.id);
              setDragging(t.id);
            }}
            onDragEnd={() => setDragging("")}
          >
            <div className="task-card-top">
              <Source value={t.source} />
              <span>{t.externalId || t.id.slice(0, 10)}</span>
            </div>
            <button className="card-title" onClick={() => edit(t)}>
              {t.title}
            </button>
            <div className="task-project">{t.project || "Без проекта"}</div>
            <PriorityBadge value={t.priority} />
            <div className="task-card-bottom">
              <span
                className={
                  t.dueDate && t.dueDate < dateKey() && t.status !== "done"
                    ? "overdue"
                    : ""
                }
              >
                {dayLabel(t.dueDate)}
              </span>
              <span className="estimate">{t.estimate} ч</span>
              {t.assigneeId && (
                <Avatar
                  member={state.members.find((m) => m.id === t.assigneeId)}
                  size="small"
                />
              )}
            </div>
          </article>
        ))}
        {page.data?.total === 0 && <p className="muted pad">Нет задач</p>}
      </div>
      <TaskPagination
        data={page.data}
        onPage={page.setPage}
        disabled={page.loading}
      />
    </section>
  );
}
function TaskList({
  filters,
  edit,
  move,
  busy,
}: Actions & { filters: TaskFilters }) {
  const { state } = useData();
  const [status, setStatus] = useState("all");
  const page = useTaskPage({ ...filters, status, limit: 25 });
  return (
    <section className="panel">
      <div className="pad">
        <select
          aria-label="Фильтр по статусу"
          value={status}
          onChange={(e) => setStatus(e.target.value)}
        >
          <option value="all">Все статусы</option>
          <option value="active">Только открытые</option>
          {columns.map((s) => (
            <option key={s} value={s}>
              {statuses[s]}
            </option>
          ))}
        </select>
      </div>
      <TaskPageFeedback {...page} />
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Задача</th>
              <th>Источник</th>
              <th>Статус</th>
              <th>Приоритет</th>
              <th>Исполнитель</th>
              <th>Срок</th>
              <th>Часы</th>
            </tr>
          </thead>
          <tbody>
            {page.data?.items.map((t) => (
              <tr key={t.id}>
                <td>
                  <button className="table-title" onClick={() => edit(t)}>
                    {t.title}
                  </button>
                  <small>{t.project}</small>
                </td>
                <td>
                  <Source value={t.source} />
                </td>
                <td>
                  <select
                    aria-label={"Статус " + t.title}
                    value={t.status}
                    disabled={busy}
                    onChange={(e) => void move(t.id, e.target.value as Status)}
                  >
                    {columns.map((s) => (
                      <option key={s} value={s}>
                        {statuses[s]}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <PriorityBadge value={t.priority} />
                </td>
                <td>
                  {state.members.find((m) => m.id === t.assigneeId)?.name ||
                    "Не назначен"}
                </td>
                <td>{dayLabel(t.dueDate)}</td>
                <td>{t.estimate}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {page.data?.total === 0 && (
        <Empty
          title="Задачи не найдены"
          description="Измените фильтры или создайте новую задачу."
        />
      )}
      <TaskPagination
        data={page.data}
        onPage={page.setPage}
        disabled={page.loading}
      />
    </section>
  );
}

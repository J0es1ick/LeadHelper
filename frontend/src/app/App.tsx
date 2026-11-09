import { useCallback, useEffect, useRef, useState } from "react";
import {
  LayoutDashboard,
  CalendarDays,
  Kanban,
  Users,
  Mail as MailIcon,
  StickyNote,
  ChartNoAxesCombined,
  Plug,
  Settings as SettingsIcon,
  Search,
  Bell,
  ChevronDown,
  ArrowUpRight,
  Command,
  HelpCircle,
  CheckCircle2,
  Menu,
  X,
  Plus,
  Leaf,
  MessageCircle,
  BookOpen,
  FolderKanban,
} from "lucide-react";
import { useStore, useData } from "./store";
import { Dashboard } from "../features/dashboard/Dashboard";
import { Backlog } from "../features/tasks/Backlog";
import { Planner } from "../features/planner/Planner";
import { Team } from "../features/team/Team";
import { Projects } from "../features/projects/Projects";
import { Mail } from "../features/mail/Mail";
import { Notes } from "../features/notes/Notes";
import { Reports } from "../features/reports/Reports";
import { Integrations } from "../features/integrations/Integrations";
import { Settings } from "../features/settings/Settings";
import { TaskEditor } from "../features/tasks/TaskEditor";
import { Avatar, Empty, Modal } from "../shared/ui";
import { dayLabel, dateKey, api, endpoint } from "../shared/api";
import type { Page, Task, Message, MailPage } from "../shared/types";
import { ThemeToggle } from "./theme";
import { Notifications, useDesktopNotifications } from "./notifications";
import { BitrixWorkspace } from "../features/integrations/BitrixWorkspace";
import { RemoteWorkspace } from "../features/integrations/RemoteWorkspace";
import { UpdateNotice } from "./UpdateNotice";
import { useDebounced, useTaskPage } from "../shared/taskPages";
const nav = [
  { id: "dashboard", name: "Обзор", icon: LayoutDashboard },
  { id: "planner", name: "Мой план", icon: CalendarDays },
  { id: "backlog", name: "Бэклог и задачи", icon: Kanban },
  { id: "projects", name: "Проекты", icon: FolderKanban },
  { id: "team", name: "Загрузка команды", icon: Users },
  { id: "mail", name: "Почта", icon: MailIcon },
  { id: "bitrix", name: "Битрикс24", icon: MessageCircle },
  { id: "confluence", name: "Confluence", icon: BookOpen },
  { id: "notes", name: "Заметки", icon: StickyNote },
  { id: "reports", name: "Отчёты", icon: ChartNoAxesCombined },
];
export function App() {
  const { loading, error, state, refresh } = useStore();
  if (loading)
    return (
      <div className="app-loading">
        <div className="brand-mark">
          <Leaf size={24} />
        </div>
        <h2>LeadHelper</h2>
        <p>Собираем ваше пространство…</p>
        <span className="loader" />
      </div>
    );
  if (!state)
    return (
      <div className="app-loading">
        <h2>Не удалось открыть пространство</h2>
        <p>{error}</p>
        <button
          className="button primary"
          onClick={() => void refresh().catch(() => {})}
        >
          Попробовать ещё раз
        </button>
      </div>
    );
  return <Workspace />;
}
function Workspace() {
  const { state, toast, notify, error: connectionError } = useData();
  useDesktopNotifications(state.notifications, state.settings.demo);
  const [page, setPage] = useState<Page>(
    () => (location.hash.slice(1) || "dashboard") as Page,
  );
  const [taskModal, setTaskModal] = useState<{
    task?: Task;
    personal?: boolean;
    date?: string;
  } | null>(null);
  const [search, setSearch] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [activity, setActivity] = useState(false);
  useEffect(() => {
    const open = () => setActivity(true);
    window.addEventListener("leadhelper:notifications", open);
    return () => window.removeEventListener("leadhelper:notifications", open);
  }, []);
  const [guide, setGuide] = useState(false);
  const [sidebar, setSidebar] = useState(false);
  const [focusId, setFocusId] = useState("");
  const [focusRequest, setFocusRequest] = useState(0);
  const searchInput = useRef<HTMLInputElement>(null);
  const navigate = useCallback((next: Page) => {
    location.hash = next;
    setPage(next);
    setSidebar(false);
    setSearchOpen(false);
    setFocusId("");
  }, []);
  useEffect(() => {
    const listener = () =>
      setPage((location.hash.slice(1) || "dashboard") as Page);
    window.addEventListener("hashchange", listener);
    return () => window.removeEventListener("hashchange", listener);
  }, []);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "k") {
        e.preventDefault();
        setSearchOpen(true);
        searchInput.current?.focus();
      }
      if (e.key === "Escape") {
        setSearchOpen(false);
        setSidebar(false);
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);
  const enabled = (p: string) =>
    ["dashboard", "integrations", "settings"].includes(p) ||
    state.settings.modules[p];
  const valid =
    [
      "dashboard",
      "planner",
      "backlog",
      "projects",
      "team",
      "mail",
      "bitrix",
      "confluence",
      "notes",
      "reports",
      "integrations",
      "settings",
    ].includes(page) && enabled(page);
  const current = valid ? page : "dashboard";
  const editRequest = useRef(0);
  const [taskLoading, setTaskLoading] = useState(false);
  const edit = useCallback(
    (task?: Task, personal = false, date?: string) => {
      const request = ++editRequest.current;
      if (!task) {
        setTaskLoading(false);
        setTaskModal({ personal, date });
        return;
      }
      setTaskModal(null);
      setTaskLoading(true);
      void api<Task>("/tasks/" + endpoint(task.id))
        .then((full) => {
          if (request === editRequest.current) setTaskModal({ task: full });
        })
        .catch((e) => {
          if (request === editRequest.current) notify(e.message, true);
        })
        .finally(() => {
          if (request === editRequest.current) setTaskLoading(false);
        });
    },
    [notify],
  );
  const closeEditor = useCallback(() => {
    editRequest.current++;
    setTaskLoading(false);
    setTaskModal(null);
  }, []);
  const unread =
    state.mailStats?.unread ??
    state.messages.filter((m) => !m.read && m.folder === "inbox").length;
  const term = search.trim().toLowerCase();
  const taskSearch = useTaskPage(
    { personal: "all", search: useDebounced(term), limit: 5 },
    !!term &&
      (state.settings.modules.backlog || state.settings.modules.planner),
  );
  const foundTasks = term ? taskSearch.data?.items || [] : [];
  const foundNotes = term
    ? state.notes
        .filter((n) => (n.title + " " + n.body).toLowerCase().includes(term))
        .slice(0, 3)
    : [];
  const [foundMessages, setFoundMessages] = useState<Message[]>([]);
  useEffect(() => {
    setFoundMessages([]);
    if (!term || !state.settings.modules.mail) return;
    const c = new AbortController();
    const timer = setTimeout(() => {
      void api<MailPage>(
        "/mail-threads?folder=all&limit=5&search=" + endpoint(term),
        "GET",
        undefined,
        c.signal,
      )
        .then((p) => setFoundMessages(p.items.map((t) => t.latest)))
        .catch(() => {});
    }, 300);
    return () => {
      clearTimeout(timer);
      c.abort();
    };
  }, [
    term,
    state.settings.demo,
    state.settings.modules.mail,
    state.mailStats?.revision,
  ]);
  const pageName =
    nav.find((n) => n.id === current)?.name ||
    (current === "settings" ? "Настройки" : "Интеграции");
  return (
    <div className="app-shell">
      <aside className={"sidebar " + (sidebar ? "open" : "")}>
        <a className="brand" href="#dashboard">
          <span className="brand-mark">
            <Leaf size={24} />
          </span>
          LeadHelper
          <span className="brand-dot" />
        </a>
        <button
          className="workspace-picker"
          onClick={() => navigate("settings")}
        >
          <span className="workspace-letter">
            {state.settings.workspace[0]}
          </span>
          <span>
            <strong>{state.settings.workspace}</strong>
            <small>Рабочее пространство</small>
          </span>
          <ChevronDown size={15} />
        </button>
        <div className="nav-section-label">ПРОСТРАНСТВО</div>
        <nav>
          {nav
            .filter((n) => enabled(n.id))
            .map((n) => (
              <button
                key={n.id}
                className={"nav-item " + (current === n.id ? "active" : "")}
                onClick={() => navigate(n.id as Page)}
              >
                <n.icon size={19} />
                <span>{n.name}</span>
                {n.id === "mail" && unread > 0 && (
                  <span className="nav-badge">{unread}</span>
                )}
                {current === n.id && <i />}
              </button>
            ))}
        </nav>
        <div className="nav-divider" />
        <div className="nav-section-label">УПРАВЛЕНИЕ</div>
        <button
          className={"nav-item " + (current === "integrations" ? "active" : "")}
          onClick={() => navigate("integrations")}
        >
          <Plug size={19} />
          <span>Интеграции</span>
          <span className="tiny-status" />
        </button>
        <button
          className={"nav-item " + (current === "settings" ? "active" : "")}
          onClick={() => navigate("settings")}
        >
          <SettingsIcon size={19} />
          <span>Настройки</span>
        </button>
        <div className="sidebar-bottom">
          <div className="help-card">
            <span className="help-icon">
              <HelpCircle size={18} />
            </span>
            <strong>
              Меньше рутины.
              <br />
              Больше фокуса.
            </strong>
            <p>
              Ваш рабочий контекст
              <br />
              теперь в одном месте.
            </p>
            <button onClick={() => setGuide(true)}>
              Как всё устроено
              <ArrowUpRight size={15} />
            </button>
          </div>
          <button className="profile" onClick={() => navigate("settings")}>
            <Avatar name={state.settings.name} />
            <span>
              <strong>{state.settings.name}</strong>
              <small>Тимлид · личное пространство</small>
            </span>
            <ChevronDown size={14} />
          </button>
        </div>
      </aside>
      {sidebar && (
        <button
          className="sidebar-scrim"
          aria-label="Закрыть меню"
          onClick={() => setSidebar(false)}
        />
      )}
      <div className="app-body">
        <header className="topbar">
          <div className="breadcrumb">
            <button
              className="icon-button mobile-menu"
              onClick={() => setSidebar(!sidebar)}
              aria-label="Открыть меню"
            >
              <Menu size={21} />
            </button>
            <span>Пространство</span>
            <span className="breadcrumb-slash">/</span>
            <strong>{pageName}</strong>
          </div>
          <div className="topbar-right">
            <div className="global-search">
              <Search size={17} />
              <input
                ref={searchInput}
                placeholder="Найти в пространстве"
                value={search}
                onFocus={() => setSearchOpen(true)}
                onChange={(e) => setSearch(e.target.value)}
                aria-label="Глобальный поиск"
              />
              <kbd>
                <Command size={11} /> K
              </kbd>
              {searchOpen && (
                <div className="search-popover">
                  <div className="search-popover-title">
                    {term ? "Результаты поиска" : "Задачи, письма и заметки"}
                    <button
                      className="icon-button"
                      aria-label="Закрыть поиск"
                      onClick={() => setSearchOpen(false)}
                    >
                      <X size={15} />
                    </button>
                  </div>
                  {!term ? (
                    <p className="muted pad">
                      Начните вводить название или текст.
                    </p>
                  ) : !foundTasks.length &&
                    !foundNotes.length &&
                    !foundMessages.length ? (
                    <Empty title="Ничего не найдено" />
                  ) : (
                    <>
                      {foundTasks.map((t) => (
                        <button
                          className="search-result"
                          key={t.id}
                          onClick={() => {
                            edit(t);
                            setSearchOpen(false);
                          }}
                        >
                          <Kanban size={17} />
                          <span>
                            {t.title}
                            <small>Задача · {t.project}</small>
                          </span>
                        </button>
                      ))}
                      {foundNotes.map((n) => (
                        <button
                          className="search-result"
                          key={n.id}
                          onClick={() => {
                            navigate("notes");
                            setFocusId(n.id);
                          }}
                        >
                          <StickyNote size={17} />
                          <span>
                            {n.title}
                            <small>Заметка</small>
                          </span>
                        </button>
                      ))}
                      {foundMessages.map((m) => (
                        <button
                          className="search-result"
                          key={m.id}
                          onClick={() => {
                            navigate("mail");
                            setFocusId(m.id);
                          }}
                        >
                          <MailIcon size={17} />
                          <span>
                            {m.subject}
                            <small>{m.fromName}</small>
                          </span>
                        </button>
                      ))}
                    </>
                  )}
                </div>
              )}
            </div>
            <span
              className={"mode-badge " + (state.settings.demo ? "" : "real")}
            >
              <i />
              {state.settings.demo ? "Деморежим" : "Рабочий режим"}
            </span>
            <button
              className="notification-button"
              aria-label={
                "Уведомления" +
                ((state.notifications || []).some((n) => !n.read)
                  ? " · есть новые"
                  : "")
              }
              onClick={() => setActivity(true)}
            >
              <Bell size={20} />
              {(state.notifications || []).some((n) => !n.read) && (
                <span className="notification-count">
                  {(state.notifications || []).filter((n) => !n.read).length}
                </span>
              )}
            </button>
            <ThemeToggle />
            <span className="topbar-divider" />
            <button
              className="profile-avatar"
              aria-label="Открыть профиль"
              onClick={() => navigate("settings")}
            >
              <Avatar name={state.settings.name} />
            </button>
          </div>
        </header>
        <main
          className={"main-content " + (current === "mail" ? "mail-page" : "")}
        >
          <UpdateNotice />
          {connectionError && (
            <div className="inline-warning" role="status">
              Не удалось обновить данные. Показана последняя загруженная версия;
              соединение восстанавливается автоматически.
            </div>
          )}
          <div className="page-context">
            <span>
              <i className="live-dot" />
              Ваше пространство готово к работе
            </span>
            <span>
              {dayLabel(dateKey(), {
                day: "numeric",
                month: "long",
                year: "numeric",
              })}
            </span>
          </div>
          {current === "dashboard" && (
            <Dashboard navigate={navigate} edit={edit} />
          )}{" "}
          {current === "backlog" && <Backlog edit={edit} />}{" "}
          {current === "planner" && <Planner edit={edit} />}{" "}
          {current === "team" && <Team />}{" "}
          {current === "projects" && <Projects />}{" "}
          {current === "mail" && <Mail focusId={focusId} />}{" "}
          {current === "bitrix" && (
            <BitrixWorkspace focusId={focusId} focusRequest={focusRequest} />
          )}
          {current === "confluence" && (
            <RemoteWorkspace
              key={current}
              provider={current}
              focusId={focusId}
            />
          )}
          {current === "notes" && <Notes focusId={focusId} />}{" "}
          {current === "reports" && <Reports edit={edit} />}{" "}
          {current === "integrations" && <Integrations />}{" "}
          {current === "settings" && (
            <Settings
              key={state.settings.demo ? "demo" : "live"}
              integrations={() => navigate("integrations")}
            />
          )}
        </main>
        <footer className="app-footer">
          <span>
            LeadHelper <span className="version">0.2.14</span>
          </span>
          <span>Создано для осмысленной работы.</span>
          <button onClick={() => setGuide(true)}>
            Помощь
            <ArrowUpRight size={13} />
          </button>
        </footer>
      </div>
      {taskLoading && (
        <Modal title="Загрузка задачи…" onClose={closeEditor}>
          <p className="pad" role="status">
            Получаем описание задачи…
          </p>
        </Modal>
      )}
      {taskModal && (
        <TaskEditor
          task={taskModal.task}
          personal={taskModal.personal}
          plannedDate={taskModal.date}
          onClose={closeEditor}
        />
      )}{" "}
      {toast && (
        <div className={"toast " + (toast.error ? "error" : "")} role="status">
          <CheckCircle2 size={18} />
          {toast.text}
        </div>
      )}
      {activity && (
        <Notifications
          close={() => setActivity(false)}
          open={(n) => {
            navigate(n.provider === "mail" ? "mail" : "bitrix");
            setFocusId(n.target);
            setFocusRequest((v) => v + 1);
          }}
        />
      )}
      {guide && (
        <Modal
          title="Добро пожаловать в LeadHelper"
          onClose={() => setGuide(false)}
        >
          <div className="form guide">
            <p>
              Пространство объединяет рабочий контекст тимлида. Начните с одного
              небольшого сценария.
            </p>
            <div>
              <Kanban size={21} />
              <span>
                <strong>Соберите задачи</strong>Откройте бэклог, добавьте
                задачу, назначьте исполнителя и оценку. Перетащите карточку в
                другой статус.
              </span>
            </div>
            <div>
              <CalendarDays size={21} />
              <span>
                <strong>Спланируйте день</strong>В «Моём плане» создайте личные
                задачи. В «Загрузке команды» нажмите на день сотрудника и
                выберите задачи, которыми он займётся в этот день.
              </span>
            </div>
            <div>
              <MailIcon size={21} />
              <span>
                <strong>Попробуйте почту</strong>Ответьте на демонстрационное
                письмо, прикрепите файл и проверьте отправленные. Реальной
                доставки в деморежиме нет.
              </span>
            </div>
            <div>
              <Plug size={21} />
              <span>
                <strong>Подключите рабочие инструменты</strong>В разделе
                интеграций есть инструкции. Ненужные модули можно отключить в
                настройках.
              </span>
            </div>
            <button className="button primary" onClick={() => setGuide(false)}>
              <Plus size={16} />
              Начать работу
            </button>
          </div>
        </Modal>
      )}
    </div>
  );
}

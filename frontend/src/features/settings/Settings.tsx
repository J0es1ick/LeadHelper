import { useEffect, useRef, useState, type FormEvent } from "react";
import { unchanged } from "../../shared/live";
import {
  CalendarDays,
  Kanban,
  Users,
  Mail,
  StickyNote,
  ChartNoAxesCombined,
  Save,
  FlaskConical,
  ShieldCheck,
  ChevronRight,
  FolderKanban,
} from "lucide-react";
import { useData } from "../../app/store";
import { api } from "../../shared/api";
import type { Settings as Config } from "../../shared/types";
import { ThemeChooser } from "../../app/theme";
import { NotificationSettings } from "../../app/notifications";
const modules = [
  {
    id: "projects",
    name: "Проекты",
    description: "Статусы проектов и импорт из таблиц",
    icon: FolderKanban,
  },
  {
    id: "planner",
    name: "Личный план",
    description: "Задачи для себя на день и неделю",
    icon: CalendarDays,
  },
  {
    id: "backlog",
    name: "Бэклог и задачи",
    description: "Единая доска командных задач",
    icon: Kanban,
  },
  {
    id: "team",
    name: "Загрузка команды",
    description: "Доступность участников и планирование",
    icon: Users,
  },
  {
    id: "mail",
    name: "Почта",
    description: "Входящие, черновики, письма и вложения",
    icon: Mail,
  },
  {
    id: "notes",
    name: "Личные заметки",
    description: "Идеи, договорённости, подготовка к встречам",
    icon: StickyNote,
  },
  {
    id: "reports",
    name: "Отчёты",
    description: "Сводки по задачам и выгрузка данных",
    icon: ChartNoAxesCombined,
  },
];
export function Settings({ integrations }: { integrations: () => void }) {
  const { state, act } = useData();
  const [form, setFormValue] = useState<Config>({
    ...state.settings,
    email: state.mailConnection?.email || state.settings.email,
  });
  const [busy, setBusy] = useState(false);
  const dirty = useRef(false),
    editVersion = useRef(0);
  function setForm(value: Config) {
    dirty.current = true;
    editVersion.current++;
    setFormValue(value);
  }
  useEffect(() => {
    if (!dirty.current && !busy)
      setFormValue((current) =>
        unchanged(current, {
          ...state.settings,
          email: state.mailConnection?.email || state.settings.email,
        }),
      );
  }, [state.settings, state.mailConnection?.email, busy]);
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    const version = editVersion.current;
    try {
      await act(() => api("/settings", "PUT", form), "Настройки сохранены");
      if (editVersion.current === version) dirty.current = false;
    } catch {
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <div className="page-heading">
        <div className="eyebrow">ПОД ВАШ РИТМ РАБОТЫ</div>
        <h1>Настройки пространства</h1>
        <p>Оставьте только то, что помогает вашей команде.</p>
      </div>
      <form className="settings-layout" onSubmit={save}>
        <div>
          <section className="panel settings-section">
            <h2>Профиль и пространство</h2>
            <p>Эти данные используются в интерфейсе и локальных черновиках.</p>
            <div className="form-grid">
              <label>
                Ваше имя
                <input
                  required
                  maxLength={120}
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                />
              </label>
              <label>
                Название пространства
                <input
                  required
                  maxLength={120}
                  value={form.workspace}
                  onChange={(e) =>
                    setForm({ ...form, workspace: e.target.value })
                  }
                />
              </label>
              <label className="full-width">
                Рабочая почта
                <input
                  required
                  type="email"
                  value={state.mailConnection?.email || form.email}
                  readOnly={!!state.mailConnection?.email}
                  onChange={(e) => setForm({ ...form, email: e.target.value })}
                />
                {state.mailConnection?.configured ? (
                  <small className="connected-mail">
                    ✓ Подключено:{" "}
                    {state.mailConnection.email || state.mailConnection.name} ·{" "}
                    {state.mailConnection.host || state.mailConnection.name}
                    <br />
                    {state.mailSync?.state === "error"
                      ? "Есть ошибка синхронизации — подробности в разделе Почта"
                      : state.mailSync?.lastSuccessAt
                        ? "Последняя синхронизация: " +
                          new Date(state.mailSync.lastSuccessAt).toLocaleString(
                            "ru-RU",
                          )
                        : "Подключение настроено"}
                    <br />
                    Адрес берётся из подключения почты.
                  </small>
                ) : (
                  <small>
                    Адрес профиля.{" "}
                    <button
                      type="button"
                      className="text-button"
                      onClick={integrations}
                    >
                      Подключить ящик
                    </button>
                  </small>
                )}
              </label>
            </div>
          </section>
          <section className="panel settings-section">
            <h2>Оформление</h2>
            <p>Тема применяется сразу и сохраняется в этом браузере.</p>
            <ThemeChooser />
          </section>
          <section className="panel settings-section">
            <h2>Оповещения</h2>
            <NotificationSettings />
          </section>
          <section className="panel settings-section">
            <h2>Функциональные модули</h2>
            <p>
              Выключенный модуль скрывается из меню. Его данные сохраняются.
            </p>
            <div className="module-list">
              {modules.map((m) => (
                <div className="module-row" key={m.id}>
                  <div className="module-icon">
                    <m.icon size={20} />
                  </div>
                  <div>
                    <h3>{m.name}</h3>
                    <p>{m.description}</p>
                  </div>
                  <button
                    type="button"
                    className={"toggle " + (form.modules[m.id] ? "on" : "")}
                    role="switch"
                    aria-checked={form.modules[m.id]}
                    aria-label={m.name}
                    onClick={() =>
                      setForm({
                        ...form,
                        modules: {
                          ...form.modules,
                          [m.id]: !form.modules[m.id],
                        },
                      })
                    }
                  >
                    <span />
                  </button>
                </div>
              ))}
            </div>
          </section>
          <section className="panel settings-section">
            <h2>Источники данных</h2>
            <p>
              Jira поставляет задачи. Битрикс24 отвечает за корпоративные
              коммуникации и события.
            </p>
            {[
              { id: "jira", name: "Jira" },
              { id: "bitrix", name: "Битрикс24" },
              { id: "confluence", name: "Confluence" },
            ].map((p) => (
              <div className="module-row" key={p.id}>
                <div className={"integration-mark " + p.id}>
                  {p.id === "jira" ? "J" : p.id === "bitrix" ? "b" : "C"}
                </div>
                <div>
                  <h3>{p.name}</h3>
                  <p>
                    {p.id === "jira"
                      ? "Импорт и обновление задач"
                      : p.id === "bitrix"
                        ? "Сообщения, новости, структура и календарь"
                        : "Пространства, страницы и поиск"}
                  </p>
                </div>
                <button
                  type="button"
                  role="switch"
                  aria-label={p.name}
                  aria-checked={form.modules[p.id]}
                  className={"toggle " + (form.modules[p.id] ? "on" : "")}
                  onClick={() =>
                    setForm({
                      ...form,
                      modules: { ...form.modules, [p.id]: !form.modules[p.id] },
                    })
                  }
                >
                  <span />
                </button>
              </div>
            ))}
            <button
              type="button"
              className="text-button margin-top"
              onClick={integrations}
            >
              Настроить подключения
              <ChevronRight size={16} />
            </button>
          </section>
        </div>
        <aside>
          <section className="demo-setting">
            <FlaskConical size={25} />
            <h2>Демонстрационный режим</h2>
            <p>
              Готовая команда, задачи, заметки и письма для знакомства с
              пространством.
            </p>
            <div className="module-row">
              <strong>{form.demo ? "Включён" : "Выключен"}</strong>
              <button
                type="button"
                role="switch"
                aria-label="Демонстрационный режим"
                aria-checked={form.demo}
                className={"toggle " + (form.demo ? "on" : "")}
                onClick={() => setForm({ ...form, demo: !form.demo })}
              >
                <span />
              </button>
            </div>
            <p className="small-text">
              При переключении открывается отдельный набор данных. Можно
              вернуться к демонстрации в любой момент. В деморежиме внешние
              запросы и доставка писем не выполняются.
            </p>
          </section>
          <section className="settings-help">
            <ShieldCheck size={21} />
            <h3>Локальное пространство</h3>
            <p>
              Контур рассчитан на одного пользователя на этом компьютере. Перед
              общим доступом потребуется добавить авторизацию и роли.
            </p>
          </section>
          <button className="button primary save-settings" disabled={busy}>
            <Save size={17} />
            {busy ? "Сохранение…" : "Сохранить настройки"}
          </button>
        </aside>
      </form>
    </>
  );
}

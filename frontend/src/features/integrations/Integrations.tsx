import { useEffect, useState, type FormEvent } from "react";
import {
  CheckCircle2,
  RefreshCw,
  ArrowUpRight,
  ShieldCheck,
  AlertCircle,
  Plug,
} from "lucide-react";
import { useData } from "../../app/store";
import { api, APIError } from "../../shared/api";
import { poll, unchanged } from "../../shared/live";
import { Modal } from "../../shared/ui";
import type { Connection, SyncLog } from "../../shared/types";
import { JiraTeamMapping } from "./JiraTeamMapping";
const descriptions: Record<string, string> = {
  jira: "Задачи вашей команды из доступных проектов. Состав и исполнители связаны с Битриксом.",
  bitrix:
    "Чаты и ответы, новости, сотрудники, отделы, файлы и занятость в календаре.",
  confluence: "Доступные пространства, страницы и поиск по базе знаний.",
  mail: "Переписки, вложения, фотографии в тексте и отправка писем через защищённый SMTP.",
};
const placeholders: Record<string, string> = {
  jira: "https://jira.example.com",
  bitrix: "https://company.bitrix24.ru",
  confluence: "https://confluence.example.com",
};
type Config = {
  url: string;
  token: string;
  email: string;
  mode: string;
  jql: string;
  scope: string;
};
export function Integrations() {
  const { state, act, notify } = useData();
  const [connections, setConnections] = useState<Connection[]>([]);
  const [busy, setBusy] = useState("");
  const [selected, setSelected] = useState<Connection | null>(null);
  const [error, setError] = useState("");
  const load = (signal?: AbortSignal) =>
    api<Connection[]>("/connections", "GET", undefined, signal)
      .then((v) => {
        if (!signal?.aborted) {
          setConnections((old) => unchanged(old, v));
          setError("");
        }
      })
      .catch((e) => {
        if (!signal?.aborted) setError(e.message);
      });
  useEffect(() => {
    const c = new AbortController();
    poll(() => load(c.signal), { signal: c.signal, interval: 10000 });
    return () => c.abort();
  }, []);
  async function sync(c: Connection) {
    setBusy(c.id);
    try {
      const l = await act(() => api<SyncLog>("/sync/" + c.id, "POST"));
      notify(l.message, !l.success);
      await load();
    } catch {
    } finally {
      setBusy("");
    }
  }
  return (
    <>
      <div className="page-heading">
        <div className="eyebrow">РАБОЧИЕ ИНСТРУМЕНТЫ</div>
        <h1>Интеграции</h1>
        <p>Ваши письма, задачи, встречи и знания — в одном пространстве.</p>
      </div>
      {state.settings.demo && (
        <div className="info-strip">
          Сейчас включён деморежим. Для загрузки рабочих данных выключите его в
          настройках.
        </div>
      )}
      {error && <div className="inline-warning">{error}</div>}
      <div className="integrations-grid">
        {connections.map((c) => {
          const active = state.settings.modules[c.id];
          const last = state.syncLogs.find((l) => l.provider === c.id);
          return (
            <section className="integration-card" key={c.id}>
              <div className="integration-card-top">
                <span className={"integration-mark large " + c.id}>
                  {{ jira: "J", bitrix: "b", confluence: "C", mail: "M" }[c.id]}
                </span>
                <span
                  className={
                    "connection-status " +
                    (active && c.configured ? "connected" : "off")
                  }
                >
                  {!active
                    ? "Отключено"
                    : c.configured
                      ? "Настроено"
                      : "Нужен доступ"}
                </span>
              </div>
              <h2>{c.name}</h2>
              <p>{descriptions[c.id]}</p>
              {c.email && (
                <div className="connected-mail">
                  {c.email}
                  <br />
                  {c.host}
                </div>
              )}
              <div className="integration-capabilities">
                {(c.id === "mail"
                  ? [
                      c.configured
                        ? "Чтение подключено"
                        : "Чтение не подключено",
                      c.sendConfigured
                        ? "Отправка подключена"
                        : "Отправка не подключена",
                    ]
                  : c.id === "jira"
                    ? [
                        "Jira Server / Data Center и Cloud",
                        "Фильтр по команде или JQL, проверка каждую минуту",
                      ]
                    : c.id === "bitrix"
                      ? [
                          "Новые сообщения: проверка каждую минуту",
                          "Календарь в разделе «Мой план»",
                        ]
                      : [
                          "Поиск во всех доступных пространствах",
                          "Страницы загружаются по запросу",
                        ]
                ).map((t) => (
                  <span key={t}>
                    <CheckCircle2 size={14} />
                    {t}
                  </span>
                ))}
              </div>
              {last && (
                <p
                  className={
                    "connection-last " + (!last.success ? "error" : "")
                  }
                >
                  {last.success ? "Последняя проверка: " : "Ошибка: "}
                  {last.success
                    ? new Date(last.at).toLocaleString("ru-RU")
                    : last.message}
                </p>
              )}
              <div className="integration-actions">
                <button className="button" onClick={() => setSelected(c)}>
                  <Plug size={15} />
                  {c.configured ? "Настройки" : "Подключить"}
                </button>
                <button
                  className="button primary"
                  disabled={
                    !!busy || !active || !c.configured || state.settings.demo
                  }
                  onClick={() => void sync(c)}
                >
                  <RefreshCw
                    size={15}
                    className={busy === c.id ? "spin" : ""}
                  />
                  {busy === c.id ? "Проверяем…" : "Синхронизировать"}
                </button>
              </div>
            </section>
          );
        })}
      </div>
      {!state.settings.demo &&
        state.settings.modules.jira &&
        connections.some((c) => c.id === "jira" && c.configured) && (
          <JiraTeamMapping />
        )}
      <section className="panel integration-history">
        <div className="section-heading">
          <h2>Последние проверки</h2>
          <span className="muted">Автоматически и по запросу</span>
        </div>
        <div className="activity-list">
          {state.syncLogs.slice(0, 15).map((l) => (
            <div key={l.id}>
              {l.success ? (
                <CheckCircle2 size={17} />
              ) : (
                <AlertCircle size={17} />
              )}
              <p>
                <strong>
                  {connections.find((c) => c.id === l.provider)?.name ||
                    l.provider}
                </strong>
                {l.message}
                <small>{new Date(l.at).toLocaleString("ru-RU")}</small>
              </p>
            </div>
          ))}
          {!state.syncLogs.length && (
            <p className="muted">
              Подключите сервис — здесь появится результат первой проверки.
            </p>
          )}
        </div>
      </section>
      {selected && (
        <ConnectionForm
          connection={selected}
          close={() => setSelected(null)}
          saved={() => {
            void load();
            setSelected(null);
          }}
        />
      )}
    </>
  );
}
function ConnectionForm({
  connection: c,
  close,
  saved,
}: {
  connection: Connection;
  close: () => void;
  saved: () => void;
}) {
  const { act } = useData();
  const [form, setForm] = useState<Config>({
    url: "",
    token: "",
    email: "",
    mode: "server",
    jql: "",
    scope: "team",
  });
  const [busy, setBusy] = useState(false);
  const [requiresUpdate, setRequiresUpdate] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (c.id !== "mail")
      void api<Config>("/connections/" + c.id)
        .then((v) =>
          setForm({
            ...v,
            url: v.url || "",
            mode: v.mode || "server",
            scope: v.scope || "team",
            token: "",
          }),
        )
        .catch((e) => {
          if (e instanceof APIError && e.status === 404) {
            setRequiresUpdate(true);
            setError(
              "Для подключения требуется обновление LeadHelper. Запустите команду ниже в PowerShell под своей учётной записью Windows, затем обновите страницу.",
            );
          } else setError(e.message);
        });
  }, [c.id]);
  const change = (key: keyof Config, value: string) =>
    setForm((f) => ({ ...f, [key]: value }));
  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await act(
        () => api("/connections/" + c.id, "PUT", form),
        "Подключение проверено и сохранено",
      );
      saved();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal title={"Подключение · " + c.name} onClose={busy ? () => {} : close}>
      {c.id === "mail" ? (
        <div className="form">
          <div className="info-strip">
            {c.email || "Почта ещё не подключена"}
          </div>
          <p>
            {c.sendConfigured
              ? "Отправка работает через SMTP. Отправленные письма сохраняются в LeadHelper и в серверной папке «Отправленные». Изображения и файлы передаются вместе с письмом."
              : "Для Timeweb отправка включается вместе с существующим подключением IMAP после обновления сервера."}
          </p>
          <p>
            Если нужно сменить ящик, запустите{" "}
            <code>scripts/configure-mail.ps1</code> на этом компьютере. Для
            другого провайдера задайте SMTP_HOST, SMTP_PORT и SMTP_SECURITY на
            сервере.
          </p>
          <p className="muted">
            Отметки прочтения и перемещения писем пока сохраняются локально.
          </p>
          <div className="modal-footer">
            <button className="button primary" onClick={close}>
              Понятно
            </button>
          </div>
        </div>
      ) : (
        <form className="form connection-form" onSubmit={submit}>
          {c.id === "bitrix" ? (
            <>
              <p>
                Создайте входящий вебхук от своего пользователя в разделе
                «Разработчикам» → «Другое» → «Входящий вебхук».
              </p>
              <div className="info-strip">
                Разрешения: чаты (im), лента новостей (log), пользователи
                (user), структура компании (department), календарь (calendar),
                диск (disk).
              </div>
              <label>
                URL входящего вебхука
                <input
                  type="password"
                  autoComplete="new-password"
                  spellCheck={false}
                  value={form.token}
                  onChange={(e) => change("token", e.target.value)}
                  placeholder={
                    c.configured
                      ? "Оставьте пустым, чтобы сохранить текущий"
                      : "https://company.bitrix24.ru/rest/…"
                  }
                  required={!c.configured}
                />
              </label>
              <p className="muted">
                Доступность методов зависит от прав пользователя и тарифа
                Битрикс24.
              </p>
            </>
          ) : (
            <>
              <label>
                Адрес сервера
                <input
                  type="url"
                  value={form.url}
                  required
                  onChange={(e) => change("url", e.target.value)}
                  placeholder={placeholders[c.id]}
                />
              </label>
              {c.id === "jira" && (
                <label>
                  Тип Jira
                  <select
                    value={form.mode}
                    onChange={(e) => change("mode", e.target.value)}
                  >
                    <option value="server">Server / Data Center</option>
                    <option value="cloud">Cloud · atlassian.net</option>
                  </select>
                </label>
              )}
              <p>
                Откройте профиль в {c.name} → «Персональные токены доступа» и
                создайте токен. Вставьте его в поле ниже.
              </p>
              <label>
                Персональный токен
                <input
                  type="password"
                  autoComplete="new-password"
                  value={form.token}
                  onChange={(e) => change("token", e.target.value)}
                  required={!c.configured}
                  placeholder={
                    c.configured
                      ? "Оставьте пустым, чтобы сохранить текущий"
                      : "Токен доступа"
                  }
                />
              </label>
              {c.id === "jira" && form.mode === "cloud" && (
                <label>
                  Email Atlassian
                  <input
                    type="email"
                    required
                    value={form.email}
                    onChange={(e) => change("email", e.target.value)}
                  />
                </label>
              )}
              {c.id === "jira" && (
                <label>
                  Какие задачи загружать
                  <select
                    value={form.scope}
                    onChange={(e) => change("scope", e.target.value)}
                  >
                    <option value="team">Команда из Битрикса</option>
                    <option value="jql">Собственная выборка по JQL</option>
                  </select>
                  <small>
                    В режиме команды загружаются только задачи её действующих
                    сотрудников. Пользователи сопоставляются по email или
                    вручную.
                  </small>
                </label>
              )}
              {c.id === "jira" && (
                <label>
                  {form.scope === "team"
                    ? "Дополнительный фильтр JQL · необязательно"
                    : "Фильтр JQL · обязательно"}
                  <textarea
                    rows={2}
                    value={form.jql}
                    onChange={(e) => change("jql", e.target.value)}
                    required={form.scope === "jql"}
                    placeholder={
                      form.scope === "team"
                        ? "Пусто — все задачи только вашей команды"
                        : "Например: project = PROJECT"
                    }
                  />
                  <small>
                    {form.scope === "team"
                      ? "JQL дополнительно сужает выборку команды. Задачи других исполнителей не загружаются."
                      : "Выборка определяется вашим JQL; автоматический фильтр команды не применяется."}
                  </small>
                </label>
              )}
            </>
          )}
          {form.url && (
            <a
              className="text-button"
              href={form.url}
              target="_blank"
              rel="noreferrer"
            >
              Открыть {c.name}
              <ArrowUpRight size={15} />
            </a>
          )}
          <div className="credential-note">
            <ShieldCheck size={17} />
            <span>
              Токен шифруется на этом компьютере с защитой вашей учётной записью
              Windows. Сохранённый токен не возвращается в браузер.
            </span>
          </div>
          {error && (
            <div className="inline-warning" role="alert">
              {error}
              {requiresUpdate && (
                <p>
                  <code>{String.raw`& C:\git\LeadHelper\scripts\update.ps1`}</code>
                </p>
              )}
            </div>
          )}
          <div className="modal-footer">
            <button
              className="button"
              type="button"
              disabled={busy}
              onClick={close}
            >
              Отмена
            </button>
            <button
              className="button primary"
              disabled={busy || requiresUpdate}
            >
              {busy ? "Проверяем доступ…" : "Проверить и подключить"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}

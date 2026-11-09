import { Fragment, useEffect, useRef, useState } from "react";
import { poll, unchanged } from "../../shared/live";
import {
  Archive,
  ChevronLeft,
  ChevronRight,
  FileUp,
  Inbox,
  Mail as MailIcon,
  Paperclip,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Send,
  Star,
  Trash2,
} from "lucide-react";
import { useData } from "../../app/store";
import { api, endpoint } from "../../shared/api";
import { Avatar, Empty } from "../../shared/ui";
import type {
  Attachment,
  Message,
  MailPage,
  MailThread,
  SyncLog,
} from "../../shared/types";
import { AttachmentPreview } from "./AttachmentPreview";
import { Composer } from "./Composer";
import { Conversation } from "./Conversation";
import { dayKey, dayLabel } from "./dates";
import { escapeHTML, plainHTML } from "./html";
import "./mail.css";
const folders = [
  { id: "inbox", label: "Входящие", icon: Inbox },
  { id: "starred", label: "Избранное", icon: Star },
  { id: "sent", label: "Отправленные", icon: Send },
  { id: "drafts", label: "Черновики", icon: Pencil },
  { id: "archive", label: "Архив", icon: Archive },
  { id: "trash", label: "Корзина", icon: Trash2 },
];
export function Mail({ focusId }: { focusId?: string }) {
  const { state, act, notify } = useData();
  const [folder, setFolder] = useState("inbox"),
    [search, setSearch] = useState(""),
    [query, setQuery] = useState(""),
    [size, setSize] = useState(25),
    [page, setPage] = useState(0),
    [cursors, setCursors] = useState([""]);
  const [data, setData] = useState<MailPage | null>(null),
    [loading, setLoading] = useState(false),
    [error, setError] = useState(""),
    [retry, setRetry] = useState(0),
    [selected, setSelected] = useState<MailThread | null>(null);
  const [compose, setCompose] = useState<{
      message: Partial<Message>;
      sourceId?: string;
    } | null>(null),
    [syncing, setSyncing] = useState(false),
    [preview, setPreview] = useState<{
      messageId: string;
      file: Attachment;
    } | null>(null);
  const input = useRef<HTMLInputElement>(null);
  const cursor = cursors[page];
  function resetPage() {
    setRetry((n) => n + 1);
    setPage(0);
    setCursors([""]);
    setData(null);
  }
  useEffect(() => {
    if (search.trim() === query) return;
    const timer = setTimeout(() => {
      setQuery(search.trim());
      resetPage();
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);
  useEffect(() => {
    const c = new AbortController();
    setLoading(true);
    setError("");
    poll(
      () =>
        api<MailPage>(
          `/mail-threads?folder=${endpoint(folder)}&search=${endpoint(query)}&limit=${size}&cursor=${endpoint(cursor)}`,
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
  }, [folder, query, size, cursor, state.mailStats?.revision, retry]);
  useEffect(() => {
    if (!focusId) return;
    const c = new AbortController();
    api<Message>("/mail/" + endpoint(focusId), "GET", undefined, c.signal)
      .then((m) => {
        setFolder(m.folder);
        resetPage();
        if (m.folder === "drafts") setCompose({ message: m });
        else
          setSelected({
            id: m.id,
            latest: m,
            count: 1,
            unread: m.read ? 0 : 1,
            starred: m.starred,
            hasAttachments: !!m.attachments?.length,
          });
      })
      .catch((e) => {
        if (e.name !== "AbortError") notify(e.message, true);
      });
    return () => c.abort();
  }, [focusId]);
  async function open(t: MailThread) {
    if (t.latest.folder === "drafts") {
      try {
        setCompose({
          message: await api<Message>("/mail/" + endpoint(t.latest.id)),
        });
      } catch (e) {
        notify((e as Error).message, true);
      }
      return;
    }
    setSelected(t);
  }
  const current = data?.items.find((t) => t.id === selected?.id) || selected;
  async function patch(body: object, close = false) {
    if (!current) return;
    try {
      await act(() =>
        api(
          `/mail-threads/${endpoint(current.id)}?folder=${endpoint(folder)}`,
          "PATCH",
          body,
        ),
      );
      if (close) setSelected(null);
    } catch {}
  }
  async function sync() {
    setSyncing(true);
    try {
      const log = await act(() => api<SyncLog>("/sync/mail", "POST"));
      notify(log.message, !log.success);
    } catch {
    } finally {
      setSyncing(false);
    }
  }
  async function importFile(file?: File) {
    if (!file) return;
    if (file.size > 7 * 1024 * 1024) {
      notify("Файл должен быть до 7 МБ", true);
      return;
    }
    try {
      await act(async () => {
        const r = await fetch("/api/mail/import", {
          method: "POST",
          headers: { "X-LeadHelper": "1", "Content-Type": "message/rfc822" },
          body: file,
        });
        if (!r.ok) throw Error((await r.json()).error);
        return r.json();
      }, "Письмо импортировано");
      setFolder("inbox");
      resetPage();
    } catch {}
    if (input.current) input.current.value = "";
  }
  function reply(m: Message, forward = false) {
    const own = state.mailConnection?.email || state.settings.email;
    setCompose({
      sourceId: forward ? m.id : undefined,
      message: {
        to: forward
          ? ""
          : m.from.toLowerCase() === own.toLowerCase()
            ? m.to
            : m.from,
        subject:
          (forward ? "Fwd: " : /^re:/i.test(m.subject) ? "" : "Re: ") +
          m.subject,
        body: "\n\n" + m.from + "\n" + m.body,
        html: forward
          ? m.html
          : '<p><br></p><blockquote data-mail-quote="block"><p>' +
            escapeHTML(
              m.fromName +
                " <" +
                m.from +
                "> " +
                new Date(m.date).toLocaleString("ru-RU"),
            ) +
            "</p>" +
            plainHTML(m.body) +
            "</blockquote>",
        attachments: forward ? m.attachments : [],
        inReplyTo: forward ? undefined : m.messageId,
        references: forward
          ? undefined
          : [m.references, m.messageId].filter(Boolean).join(" "),
      },
    });
  }
  return (
    <>
      <div className="page-heading heading-flex">
        <div>
          <div className="eyebrow">РАБОЧАЯ КОММУНИКАЦИЯ</div>
          <h1>Почта</h1>
          <p>Важные разговоры рядом с задачами и планами.</p>
        </div>
        <button
          className="button"
          disabled={syncing || state.mailSync?.state === "syncing"}
          onClick={() => void sync()}
        >
          <RefreshCw size={16} className={syncing ? "spin" : ""} />
          {syncing ? "Загрузка…" : "Получить почту"}
        </button>
      </div>
      {state.settings.demo ? (
        <div className="demo-notice">
          <span className="live-dot" />
          Демонстрационная почта. Письма остаются внутри LeadHelper.
        </div>
      ) : (
        <details className="mail-sync-panel">
          <summary>
            <strong>
              {state.mailSync?.state === "syncing"
                ? "Получаем почту…"
                : state.mailSync?.state === "error"
                  ? "Есть ошибка синхронизации"
                  : state.mailConnection?.configured
                    ? "Почта подключена"
                    : "Ожидаем подключения почты"}
            </strong>{" "}
            · {state.mailConnection?.email || state.settings.email}
          </summary>
          <p>
            {state.mailSync?.error ||
              "Почта проверяется автоматически, в том числе после простоя компьютера."}
          </p>
          {state.mailSync?.lastSuccessAt && (
            <small>
              Последняя проверка:{" "}
              {new Date(state.mailSync.lastSuccessAt).toLocaleString("ru-RU")}
            </small>
          )}
          <small>
            Отметки и перемещения сохраняются локально.{" "}
            {state.mailConnection?.sendConfigured
              ? "Отправка через SMTP подключена."
              : "Отправка пока не подключена."}{" "}
            <a href="#integrations">Настройки подключения</a>
          </small>
        </details>
      )}
      <div className="mail-layout">
        <aside className="mail-folders">
          <button
            className="button primary compose-button"
            onClick={() => setCompose({ message: {} })}
          >
            <Plus size={17} />
            Написать
          </button>
          {folders.map((f) => (
            <button
              className={"folder " + (folder === f.id ? "active" : "")}
              key={f.id}
              onClick={() => {
                setFolder(f.id);
                setSelected(null);
                resetPage();
              }}
            >
              <f.icon size={17} />
              <span>{f.label}</span>
              <small>{state.mailStats?.folders[f.id] ?? 0}</small>
            </button>
          ))}
          <div className="mail-account">
            <Avatar name={state.settings.name} />
            <span>
              {state.settings.name}
              <small>
                {state.mailConnection?.email || state.settings.email}
              </small>
            </span>
          </div>
          <button
            className="import-button"
            onClick={() => input.current?.click()}
          >
            <FileUp size={16} />
            Импорт .eml
          </button>
          <input
            hidden
            type="file"
            ref={input}
            accept=".eml,message/rfc822"
            onChange={(e) => void importFile(e.target.files?.[0])}
          />
        </aside>
        <section className="mail-list" aria-busy={loading}>
          <div className="mail-list-heading">
            <h3>{folders.find((f) => f.id === folder)?.label}</h3>
            <span className="count-badge">
              {state.mailStats?.folders[folder] ?? 0}
            </span>
          </div>
          <div className="search-field">
            <Search size={16} />
            <input
              placeholder="Поиск в этой папке…"
              aria-label="Поиск писем"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <div className="mail-items">
            {error ? (
              <div className="mail-load-error" role="alert">
                {error}
                <button
                  className="button"
                  onClick={() => setRetry((n) => n + 1)}
                >
                  Повторить
                </button>
              </div>
            ) : !data ? (
              <p className="mail-loading" role="status">
                Загружаем переписки…
              </p>
            ) : data.items.length ? (
              data.items.map((t, i) => {
                const m = t.latest;
                return (
                  <Fragment key={t.id}>
                    {(i === 0 ||
                      dayKey(data.items[i - 1].latest.date) !==
                        dayKey(m.date)) && (
                      <h4 className="mail-day">{dayLabel(m.date)}</h4>
                    )}
                    <button
                      className={
                        "mail-item " +
                        (current?.id === t.id ? "selected " : "") +
                        (t.unread ? "unread" : "")
                      }
                      aria-label={
                        (t.unread
                          ? "Непрочитанная переписка: "
                          : "Переписка: ") + m.subject
                      }
                      onClick={() => void open(t)}
                    >
                      <div>
                        <strong>
                          {m.folder === "sent" || m.folder === "drafts"
                            ? m.to || "Без получателя"
                            : m.fromName || m.from}
                        </strong>
                        <time>
                          {new Date(m.date).toLocaleTimeString("ru-RU", {
                            hour: "2-digit",
                            minute: "2-digit",
                          })}
                        </time>
                      </div>
                      <h4>
                        {m.subject || "Без темы"}
                        {t.starred && <Star size={12} fill="currentColor" />}
                      </h4>
                      <p>{m.body}</p>
                      <div className="thread-meta">
                        <span>{t.count > 1 ? `${t.count} писем` : ""}</span>
                        {t.hasAttachments && <Paperclip size={13} />}
                      </div>
                      {!!t.unread && (
                        <i
                          className="unread-dot thread-unread-dot"
                          aria-hidden="true"
                        />
                      )}
                    </button>
                  </Fragment>
                );
              })
            ) : (
              <Empty
                title={query ? "Ничего не найдено" : "Переписок нет"}
                description={
                  query
                    ? "Попробуйте другое слово или адрес отправителя."
                    : "Здесь появятся письма этой папки."
                }
              />
            )}
          </div>
          <nav className="mail-pagination" aria-label="Страницы писем">
            <button
              className="icon-button"
              aria-label="Предыдущая страница писем"
              disabled={page === 0 || loading}
              onClick={() => {
                setData(null);
                setPage((n) => n - 1);
              }}
            >
              <ChevronLeft size={19} />
            </button>
            <span>Страница {page + 1}</span>
            <button
              className="icon-button"
              aria-label="Следующая страница писем"
              disabled={!data?.nextCursor || loading}
              onClick={() => {
                setCursors((c) => [...c.slice(0, page + 1), data!.nextCursor!]);
                setData(null);
                setPage((n) => n + 1);
              }}
            >
              <ChevronRight size={19} />
            </button>
            <select
              aria-label="Переписок на странице"
              value={size}
              onChange={(e) => {
                setSize(Number(e.target.value));
                resetPage();
              }}
            >
              {[25, 50, 100].map((n) => (
                <option key={n} value={n}>
                  {n} на стр.
                </option>
              ))}
            </select>
          </nav>
        </section>
        <section className="mail-reader">
          {current ? (
            <>
              <div className="mail-actions">
                <button
                  className={
                    "icon-button " + (current.starred ? "starred" : "")
                  }
                  aria-label="Отметить избранным"
                  onClick={() => void patch({ starred: !current.starred })}
                >
                  <Star
                    size={18}
                    fill={current.starred ? "currentColor" : "none"}
                  />
                </button>
                <button
                  className="icon-button"
                  aria-label="Архивировать"
                  onClick={() => void patch({ folder: "archive" }, true)}
                >
                  <Archive size={18} />
                </button>
                <button
                  className="icon-button"
                  aria-label={
                    folder === "trash"
                      ? "Восстановить"
                      : "Переместить в корзину"
                  }
                  onClick={() =>
                    void patch(
                      { folder: folder === "trash" ? "inbox" : "trash" },
                      true,
                    )
                  }
                >
                  {folder === "trash" ? (
                    <Inbox size={18} />
                  ) : (
                    <Trash2 size={18} />
                  )}
                </button>
                <button
                  className="icon-button"
                  aria-label={
                    current.unread
                      ? "Отметить переписку прочитанной"
                      : "Отметить переписку непрочитанной"
                  }
                  onClick={() => void patch({ read: current.unread > 0 })}
                >
                  <MailIcon size={18} />
                </button>
              </div>
              <Conversation
                key={current.id + folder}
                id={current.id}
                folder={folder}
                onReply={reply}
                onRead={(thread) => {
                  setData(
                    (value) =>
                      value && {
                        ...value,
                        items: value.items.map((item) =>
                          item.id === thread.id ? thread : item,
                        ),
                      },
                  );
                  setSelected((value) =>
                    value && (value.id === thread.id || value.id === current.id)
                      ? thread
                      : value,
                  );
                }}
                onPreview={(messageId, file) => setPreview({ messageId, file })}
              />
            </>
          ) : (
            <Empty
              title="Каждое письмо на своём месте"
              description="Выберите переписку. Последнее сообщение откроется сверху, остальные можно развернуть."
            />
          )}
        </section>
      </div>
      {compose && (
        <Composer
          initial={compose.message}
          sourceId={compose.sourceId}
          onClose={() => setCompose(null)}
        />
      )}
      {preview && (
        <AttachmentPreview
          messageId={preview.messageId}
          file={preview.file}
          onClose={() => setPreview(null)}
        />
      )}
    </>
  );
}

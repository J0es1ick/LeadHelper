import { Fragment, useEffect, useRef, useState } from "react";
import { poll, unchanged } from "../../shared/live";
import {
  ChevronDown,
  ChevronRight,
  Download,
  Eye,
  Forward,
  Paperclip,
  Reply,
} from "lucide-react";
import { useData } from "../../app/store";
import { api, endpoint } from "../../shared/api";
import { Avatar } from "../../shared/ui";
import type {
  Attachment,
  MailThread,
  Message,
  MessagePage,
} from "../../shared/types";
import { attachmentURL, formatFileSize } from "./AttachmentPreview";
import { MessageBody } from "./MessageBody";
import { dayKey, dayLabel } from "./dates";

export function Conversation({
  id,
  folder,
  onReply,
  onPreview,
  onRead,
}: {
  id: string;
  folder: string;
  onReply: (m: Message, forward?: boolean) => void;
  onPreview: (messageId: string, file: Attachment) => void;
  onRead: (thread: MailThread) => void;
}) {
  const { state, act } = useData();
  const viewed = useRef(new Set<string>());
  const [data, setData] = useState<MessagePage | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(false),
    [cursors, setCursors] = useState([""]),
    [page, setPage] = useState(0),
    [open, setOpen] = useState<Record<string, boolean>>({}),
    [retry, setRetry] = useState(0);
  const cursor = cursors[page];
  async function markViewed(messageId: string) {
    if (!data || viewed.current.has(messageId)) return;
    viewed.current.add(messageId);
    if (!data.thread.unread) return;
    try {
      await act(async () => {
        await api(
          `/mail-threads/${endpoint(data.thread.id)}?folder=${endpoint(folder)}`,
          "PATCH",
          { read: true },
        );
        // Update the list immediately after persistence, without waiting for polling.
        onRead({
          ...data.thread,
          unread: 0,
          latest: { ...data.thread.latest, read: true },
        });
      });
    } catch {
      viewed.current.delete(messageId);
    }
  }
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    poll(
      () =>
        api<MessagePage>(
          `/mail-threads/${endpoint(id)}?folder=${endpoint(folder)}&limit=20&cursor=${endpoint(cursor)}`,
          "GET",
          undefined,
          controller.signal,
        )
          .then((v) => {
            if (!controller.signal.aborted) {
              setData((old) => unchanged(old, v));
              setError("");
            }
          })
          .catch((e) => {
            if (!controller.signal.aborted) setError(e.message);
            throw e;
          })
          .finally(() => {
            if (!controller.signal.aborted) setLoading(false);
          }),
      { signal: controller.signal, interval: 10000 },
    );
    return () => controller.abort();
  }, [id, folder, cursor, state.mailStats?.revision, retry]);
  if (error)
    return (
      <div className="mail-load-error" role="alert">
        {error}
        <button className="button" onClick={() => setRetry((n) => n + 1)}>
          Повторить
        </button>
      </div>
    );
  if (!data)
    return (
      <p className="mail-loading" role="status">
        Загружаем переписку…
      </p>
    );
  return (
    <div className="mail-content" aria-busy={loading}>
      <h2>{data.thread.latest.subject || "Без темы"}</h2>
      <p className="thread-caption">{data.thread.count} писем · новые сверху</p>
      {data.items.map((m, i, list) => (
        <Fragment key={m.id}>
          {(i === 0 || dayKey(list[i - 1].date) !== dayKey(m.date)) && (
            <h3 className="conversation-day">{dayLabel(m.date)}</h3>
          )}
          <MessageCard
            header={m}
            expanded={open[m.id] ?? (page === 0 && i === 0)}
            toggle={() =>
              setOpen((v) => ({
                ...v,
                [m.id]: !(v[m.id] ?? (page === 0 && i === 0)),
              }))
            }
            onReply={onReply}
            onPreview={onPreview}
            onViewed={markViewed}
          />
        </Fragment>
      ))}
      {(page > 0 || data.nextCursor) && (
        <nav
          className="mail-pagination thread-pagination"
          aria-label="Страницы переписки"
        >
          <button
            className="button"
            disabled={page === 0 || loading}
            onClick={() => {
              setData(null);
              setPage((n) => n - 1);
            }}
          >
            Более новые
          </button>
          <span>Часть {page + 1}</span>
          <button
            className="button"
            disabled={!data.nextCursor || loading}
            onClick={() => {
              setCursors((c) => [...c.slice(0, page + 1), data.nextCursor!]);
              setData(null);
              setPage((n) => n + 1);
            }}
          >
            Более ранние
          </button>
        </nav>
      )}
    </div>
  );
}
function MessageCard({
  header,
  expanded,
  toggle,
  onReply,
  onPreview,
  onViewed,
}: {
  header: Message;
  expanded: boolean;
  toggle: () => void;
  onReply: (m: Message, forward?: boolean) => void;
  onPreview: (id: string, file: Attachment) => void;
  onViewed: (id: string) => Promise<void>;
}) {
  const { state } = useData();
  const viewedHandler = useRef(onViewed);
  viewedHandler.current = onViewed;
  const [message, setMessage] = useState<Message | null>(null),
    [error, setError] = useState(""),
    [retry, setRetry] = useState(0);
  useEffect(() => {
    if (!expanded) return;
    const controller = new AbortController();
    setError("");
    api<Message>(
      "/mail/" + endpoint(header.id),
      "GET",
      undefined,
      controller.signal,
    )
      .then((m) => {
        if (controller.signal.aborted) return;
        setMessage(m);
        void viewedHandler.current(m.id);
      })
      .catch((e) => {
        if (e.name !== "AbortError") setError(e.message);
      });
    return () => controller.abort();
  }, [header.id, expanded, retry]);
  const own =
    header.from.toLowerCase() ===
    (state.mailConnection?.email || state.settings.email).toLowerCase();
  return (
    <article
      className={
        "conversation-message " +
        (own ? "outgoing " : "") +
        (expanded ? "expanded" : "collapsed")
      }
    >
      <button
        className="message-summary"
        aria-expanded={expanded}
        aria-label={
          (expanded ? "Свернуть письмо: " : "Развернуть письмо: ") +
          (own ? "Вы" : header.fromName || header.from) +
          ", " +
          new Date(header.date).toLocaleString("ru-RU")
        }
        onClick={toggle}
      >
        <Avatar name={own ? "Вы" : header.fromName || header.from} />
        <span className="summary-person">
          <strong>{own ? "Вы" : header.fromName || header.from}</strong>
          {!expanded && <span>{header.body}</span>}
        </span>
        <time>
          {new Date(header.date).toLocaleTimeString("ru-RU", {
            hour: "2-digit",
            minute: "2-digit",
          })}
        </time>
        {expanded ? <ChevronDown size={17} /> : <ChevronRight size={17} />}
      </button>
      {expanded && (
        <div className="message-expanded">
          {error ? (
            <div role="alert">
              {error}
              <button
                className="text-button"
                onClick={() => setRetry((n) => n + 1)}
              >
                Повторить
              </button>
            </div>
          ) : !message ? (
            <p className="muted" role="status">
              Загружаем письмо…
            </p>
          ) : (
            <>
              <details className="message-details">
                <summary>Детали письма</summary>
                <p>
                  От: {message.from}
                  <br />
                  Кому: {message.to}
                  {message.cc && (
                    <>
                      <br />
                      Копия: {message.cc}
                    </>
                  )}
                  <br />
                  {new Date(message.date).toLocaleString("ru-RU")}
                  {message.sourceFolder && (
                    <>
                      <br />
                      Папка на сервере: {message.sourceFolder}
                    </>
                  )}
                </p>
              </details>
              {message.importWarning && (
                <div className="inline-warning">{message.importWarning}</div>
              )}
              {message.delivery === "demo" && (
                <p className="muted">
                  Тестовое письмо · адресату не доставлялось
                </p>
              )}
              <MessageBody message={message} />
              {message.attachments?.length > 0 && (
                <div className="attachments">
                  <h4>
                    <Paperclip size={15} />
                    {message.attachments.length} вложений
                  </h4>
                  {message.attachments.map((a) => (
                    <div className="attachment-card" key={a.id}>
                      <button
                        className="attachment-open"
                        aria-label={"Предпросмотр: " + a.name}
                        onClick={() => onPreview(message.id, a)}
                      >
                        <Paperclip size={19} />
                        <span>
                          <strong>{a.name}</strong>
                          <small>
                            {formatFileSize(a.size ?? a.data.length * 0.75)} ·
                            Предпросмотр
                          </small>
                        </span>
                        <Eye size={17} />
                      </button>
                      <a
                        className="icon-button"
                        href={attachmentURL(message.id, a.id)}
                        download={a.name}
                        aria-label={"Скачать " + a.name}
                      >
                        <Download size={18} />
                      </a>
                    </div>
                  ))}
                </div>
              )}
              <div className="reply-actions">
                <button className="button" onClick={() => onReply(message)}>
                  <Reply size={16} />
                  Ответить
                </button>
                <button
                  className="button"
                  onClick={() => onReply(message, true)}
                >
                  <Forward size={16} />
                  Переслать
                </button>
                <a
                  className="icon-button"
                  href={"/api/mail/" + endpoint(message.id) + "/export"}
                  aria-label="Скачать это письмо EML"
                >
                  <Download size={16} />
                </a>
              </div>
            </>
          )}
        </div>
      )}
    </article>
  );
}

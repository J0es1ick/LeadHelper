import { useEffect, useState } from "react";
import {
  ArrowLeft,
  ArrowUpRight,
  BookOpen,
  ChevronLeft,
  ChevronRight,
  FileText,
  Folder,
  MessageCircle,
  RefreshCw,
  Search,
  Send,
} from "lucide-react";
import { useData } from "../../app/store";
import { api, endpoint } from "../../shared/api";
import { poll, unchanged } from "../../shared/live";
import { Avatar, Empty, Modal } from "../../shared/ui";
import { BitrixText } from "./BitrixText";
import type { RemoteItem, RemotePage } from "../../shared/types";
import { cleanHTML } from "../mail/html";
import { ChatAttachments } from "./ChatAttachments";
import { BitrixFilePreview } from "./BitrixFilePreview";

export function RemoteWorkspace({
  provider,
  focusId = "",
}: {
  provider: "bitrix" | "confluence";
  focusId?: string;
}) {
  const { state } = useData();
  const [kind, setKind] = useState(provider === "bitrix" ? "chats" : "spaces");
  const [parent, setParent] = useState("");
  const [parentName, setParentName] = useState("");
  const [search, setSearch] = useState("");
  const [term, setTerm] = useState("");
  const [cursors, setCursors] = useState([""]);
  const [page, setPage] = useState<RemotePage>({ items: [] });
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState<RemoteItem | null>(null);
  const [reload, setReload] = useState(0);
  const cursor = cursors[cursors.length - 1];
  useEffect(() => {
    if (focusId && provider === "bitrix") {
      setSelected({ id: focusId, title: "Диалог", kind: "chat" });
      setKind("chats");
    }
  }, [focusId, provider]);
  useEffect(() => {
    const t = setTimeout(() => {
      setTerm(search);
      setCursors([""]);
    }, 350);
    return () => clearTimeout(t);
  }, [search]);
  useEffect(() => {
    if (state.settings.demo) return;
    const abort = new AbortController();
    setLoading(true);
    setError("");
    setPage({ items: [] });
    const params = new URLSearchParams({
      kind,
      id: parent,
      cursor,
      search: term,
    });
    poll(
      () =>
        api<RemotePage>(
          "/workspace/" + provider + "?" + params,
          "GET",
          undefined,
          abort.signal,
        )
          .then((v) => {
            if (!abort.signal.aborted) {
              setPage((old) => unchanged(old, v));
              setError("");
            }
          })
          .catch((e) => {
            if (!abort.signal.aborted) setError(e.message);
            throw e;
          })
          .finally(() => {
            if (!abort.signal.aborted) setLoading(false);
          }),
      { signal: abort.signal, interval: 30000 },
    );
    return () => abort.abort();
  }, [provider, kind, parent, cursor, term, reload, state.settings.demo]);
  function section(next: string) {
    setKind(next);
    setParent("");
    setParentName("");
    setSearch("");
    setTerm("");
    setCursors([""]);
    setSelected(null);
  }
  function open(item: RemoteItem) {
    if (item.kind === "space") {
      setKind("pages");
      setParent(item.id);
      setParentName(item.title);
      setCursors([""]);
    } else if (item.kind === "folder") {
      setParent(item.id);
      setParentName(item.title);
      setCursors([""]);
    } else {
      setSelected(item);
    }
  }
  const tabs =
    provider === "bitrix"
      ? [
          ["chats", "Чаты"],
          ["news", "Новости"],
          ["people", "Сотрудники"],
          ["departments", "Отделы"],
          ["files", "Файлы"],
        ]
      : [
          ["spaces", "Пространства"],
          ["pages", "Все страницы"],
        ];
  return (
    <>
      <div className="page-heading heading-flex">
        <div>
          <div className="eyebrow">
            {provider === "bitrix" ? "КОМАНДА И ОБЩЕНИЕ" : "БАЗА ЗНАНИЙ"}
          </div>
          <h1>{provider === "bitrix" ? "Битрикс24" : "Confluence"}</h1>
          <p>
            {provider === "bitrix"
              ? "Рабочие обсуждения, новости и документы команды."
              : "Все пространства, к которым у вашей учётной записи есть доступ."}
          </p>
        </div>
        <button
          className="button"
          onClick={() => setReload((x) => x + 1)}
          disabled={loading}
        >
          <RefreshCw size={16} className={loading ? "spin" : ""} />
          Обновить
        </button>
      </div>
      <div className="remote-toolbar">
        <div className="segmented">
          {tabs.map(([id, title]) => (
            <button
              key={id}
              className={kind === id ? "active" : ""}
              onClick={() => section(id)}
            >
              {title}
            </button>
          ))}
        </div>
        {provider === "confluence" && kind === "pages" && (
          <label className="remote-search">
            <Search size={17} />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Поиск по содержимому страниц"
              aria-label="Поиск Confluence"
            />
          </label>
        )}
      </div>
      {parent && (
        <button
          className="text-button remote-breadcrumb"
          onClick={() =>
            section(provider === "confluence" ? "spaces" : "files")
          }
        >
          <ArrowLeft size={16} />
          {parentName} · К списку{" "}
          {provider === "confluence" ? "пространств" : "хранилищ"}
        </button>
      )}
      {state.settings.demo ? (
        <Empty
          title="Рабочие данные в деморежиме скрыты"
          description="Отключите деморежим в настройках, затем подключите сервис."
          action={
            <a className="button" href="#settings">
              Открыть настройки
            </a>
          }
        />
      ) : error ? (
        <div className="remote-error">
          <div className="inline-warning" role="alert">
            {error}
          </div>
          <a className="button" href="#integrations">
            Настроить подключение
          </a>
        </div>
      ) : loading ? (
        <div className="remote-loading" role="status">
          <span className="loader" />
          Загружаем {provider === "bitrix" ? "из Битрикса" : "из Confluence"}…
        </div>
      ) : (
        <div
          className={"remote-grid " + (kind === "chats" ? "remote-chats" : "")}
        >
          {page.items.map((item) => (
            <button
              key={item.id}
              className={"remote-card " + (item.unread ? "unread" : "")}
              onClick={() => open(item)}
            >
              <span className="remote-icon">
                {item.kind === "chat" ? (
                  <MessageCircle size={20} />
                ) : item.kind === "folder" ? (
                  <Folder size={20} />
                ) : item.kind === "space" ? (
                  <BookOpen size={20} />
                ) : (
                  <FileText size={20} />
                )}
              </span>
              <span className="remote-card-content">
                <strong>{item.title || item.id}</strong>
                {item.text && <p>{item.text.slice(0, 280)}</p>}
                <small>
                  {item.author}
                  {item.date && !item.date.startsWith("0001")
                    ? " · " + new Date(item.date).toLocaleString("ru-RU")
                    : ""}
                </small>
              </span>
              {!!item.unread && (
                <span className="remote-unread">{item.unread}</span>
              )}
            </button>
          ))}
          {!page.items.length && (
            <Empty
              title={
                term ? "По запросу ничего не найдено" : "Здесь пока нет записей"
              }
              description="Показаны только данные, доступные вашему пользователю."
            />
          )}
        </div>
      )}
      {!error && !state.settings.demo && (
        <div className="remote-pagination">
          <button
            className="button small"
            disabled={loading || cursors.length === 1}
            onClick={() => setCursors((v) => v.slice(0, -1))}
          >
            <ChevronLeft size={16} />
            Назад
          </button>
          <span>Страница {cursors.length}</span>
          <button
            className="button small"
            disabled={loading || !page.next || page.next === cursor}
            onClick={() => setCursors((v) => [...v, page.next!])}
          >
            Далее
            <ChevronRight size={16} />
          </button>
        </div>
      )}
      {selected &&
        (selected.kind === "chat" ? (
          <Chat
            item={selected}
            close={() => {
              setSelected(null);
              setReload((x) => x + 1);
            }}
          />
        ) : selected.kind === "file" &&
          provider === "bitrix" &&
          selected.attachments?.[0] ? (
          <BitrixFilePreview
            file={selected.attachments[0]}
            onClose={() => setSelected(null)}
          />
        ) : (
          <RemoteDetail
            provider={provider}
            item={selected}
            close={() => setSelected(null)}
          />
        ))}
    </>
  );
}
function RemoteDetail({
  provider,
  item,
  close,
}: {
  provider: string;
  item: RemoteItem;
  close: () => void;
}) {
  const [content, setContent] = useState(item);
  const [loading, setLoading] = useState(item.kind === "page");
  const [error, setError] = useState("");
  useEffect(() => {
    if (item.kind !== "page") return;
    const a = new AbortController();
    poll(
      () =>
        api<RemotePage>(
          "/workspace/" + provider + "?kind=page&id=" + endpoint(item.id),
          "GET",
          undefined,
          a.signal,
        )
          .then((v) => {
            if (!a.signal.aborted) {
              setContent((old) => unchanged(old, v.items[0] || item));
              setError("");
            }
          })
          .catch((e) => {
            if (!a.signal.aborted) setError(e.message);
            throw e;
          })
          .finally(() => {
            if (!a.signal.aborted) setLoading(false);
          }),
      { signal: a.signal, interval: 30000 },
    );
    return () => a.abort();
  }, [item, provider]);
  return (
    <Modal title={content.title} onClose={close} wide>
      <div className="remote-detail">
        {loading ? (
          <p role="status">Загружаем страницу…</p>
        ) : error ? (
          <div className="inline-warning">{error}</div>
        ) : content.html ? (
          <div
            className="message-html"
            dangerouslySetInnerHTML={{ __html: cleanHTML(content.html) }}
          />
        ) : (
          <p className="remote-text">
            {content.text || "Подробности доступны в исходном сервисе."}
          </p>
        )}
        {content.url && (
          <a
            className="button"
            href={content.url}
            target="_blank"
            rel="noreferrer"
          >
            Открыть в {provider === "bitrix" ? "Битрикс24" : "Confluence"}
            <ArrowUpRight size={16} />
          </a>
        )}
      </div>
    </Modal>
  );
}
function Chat({ item, close }: { item: RemoteItem; close: () => void }) {
  const { notify } = useData();
  const [page, setPage] = useState<RemotePage>({ items: [] });
  const [cursors, setCursors] = useState([""]);
  const [error, setError] = useState("");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [reload, setReload] = useState(0);
  const [uncertain, setUncertain] = useState(false);
  const cursor = cursors[cursors.length - 1];
  useEffect(() => {
    const a = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function load() {
      try {
        setPage(
          await api<RemotePage>(
            "/workspace/bitrix?kind=messages&id=" +
              endpoint(item.id) +
              "&cursor=" +
              endpoint(cursor),
            "GET",
            undefined,
            a.signal,
          ),
        );
        setError("");
      } catch (e) {
        if (!a.signal.aborted) setError((e as Error).message);
      } finally {
        if (!a.signal.aborted) {
          setLoading(false);
          if (!cursor) timer = setTimeout(() => void load(), 15000);
        }
      }
    }
    setLoading(true);
    void load();
    return () => {
      a.abort();
      clearTimeout(timer);
    };
  }, [item.id, cursor, reload]);
  async function send() {
    if (!text.trim() || busy) return;
    setBusy(true);
    try {
      await api("/bitrix/chats/" + endpoint(item.id) + "/messages", "POST", {
        text,
      });
      setText("");
      setCursors([""]);
      setReload((x) => x + 1);
      notify("Сообщение отправлено");
    } catch (e) {
      setError((e as Error).message);
      setUncertain(true);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal title={item.title} onClose={close} wide>
      <div className="chat-detail">
        {error && (
          <div className="inline-warning" role="alert">
            {error}
          </div>
        )}
        <div className="chat-compose">
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Написать сообщение…"
            aria-label="Сообщение в Битрикс24"
            rows={3}
            maxLength={10000}
          />
          <div>
            {item.url && (
              <a
                className="text-button"
                href={item.url}
                target="_blank"
                rel="noreferrer"
              >
                Открыть чат
                <ArrowUpRight size={14} />
              </a>
            )}
            <button
              className="button primary"
              disabled={busy || !text.trim() || uncertain}
              onClick={() => void send()}
            >
              <Send size={15} />
              {busy ? "Отправляем…" : "Отправить"}
            </button>
          </div>
          {uncertain && (
            <button
              className="text-button"
              onClick={() => {
                setUncertain(false);
                setReload((x) => x + 1);
              }}
            >
              Я проверил чат — разрешить повторную отправку
            </button>
          )}
        </div>
        <div className="remote-pagination">
          <button
            className="button small"
            disabled={cursors.length === 1 || loading}
            onClick={() => setCursors((v) => v.slice(0, -1))}
          >
            Новее
          </button>
          <span>Последние сообщения сверху</span>
          <button
            className="button small"
            disabled={!page.next || loading}
            onClick={() => setCursors((v) => [...v, page.next!])}
          >
            Старее
          </button>
        </div>
        <div className="chat-messages">
          {loading ? (
            <p role="status">Загружаем сообщения…</p>
          ) : (
            page.items.map((m) => (
              <article className="chat-message" key={m.id}>
                <header>
                  <Avatar name={m.title} src={m.avatarUrl} size="small" />
                  <strong>{m.title}</strong>
                  <time>
                    {m.date ? new Date(m.date).toLocaleString("ru-RU") : ""}
                  </time>
                </header>
                <BitrixText text={m.text} />
                <ChatAttachments files={m.attachments} />
              </article>
            ))
          )}
        </div>
      </div>
    </Modal>
  );
}

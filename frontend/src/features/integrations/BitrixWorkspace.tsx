import {
  Fragment,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  ArrowLeft,
  ArrowUpRight,
  ChevronLeft,
  ChevronRight,
  Folder,
  MessageCircle,
  Network,
  Newspaper,
  Paperclip,
  RefreshCw,
  Search,
  Send,
  Users,
  X,
} from "lucide-react";
import { useData } from "../../app/store";
import { api, endpoint } from "../../shared/api";
import { poll, unchanged } from "../../shared/live";
import { Avatar, Empty } from "../../shared/ui";
import { BitrixText } from "./BitrixText";
import type { RemoteItem, RemotePage } from "../../shared/types";
import { dayKey, dayLabel } from "../mail/dates";
import {
  departmentTree,
  plainRemoteText,
  sortMessages,
  type DepartmentNode,
} from "./bitrix-model";
import "./bitrix.css";
import { ChatAttachments } from "./ChatAttachments";
import { DialogCache, bitrixPage } from "./bitrix-cache";
import { ChatCards } from "./ChatCards";
import { ChatFileDrafts } from "./ChatFileDrafts";
import {
  addChatFiles,
  uploadChatFile,
  ChatUploadError,
  type ChatFileDraft,
  type FileDraftUpdate,
} from "./chat-upload";

type Kind = "chats" | "news" | "people" | "departments" | "files";
function useRemote(
  kind: string,
  params: Record<string, string>,
  refresh: number,
) {
  const key = JSON.stringify(params);
  const [position, setPosition] = useState({ key, cursors: [""] });
  const cursors = position.key === key ? position.cursors : [""];
  const cursor = cursors[cursors.length - 1];
  const [data, setData] = useState<RemotePage>({ items: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const loadedKey = useRef("");
  useEffect(() => {
    setPosition((v) => (v.key === key ? v : { key, cursors: [""] }));
  }, [key]);
  useEffect(() => {
    const abort = new AbortController();
    const requestKey = kind + key + cursor;
    setLoading(loadedKey.current !== requestKey);
    setError("");
    const q = new URLSearchParams({ ...JSON.parse(key), kind, cursor });
    async function load() {
      await bitrixPage("/workspace/bitrix?" + q, abort.signal)
        .then((value) => {
          if (!abort.signal.aborted) {
            setData((old) => unchanged(old, value));
            loadedKey.current = requestKey;
            setError("");
          }
        })
        .catch((e) => {
          if (!abort.signal.aborted) setError(e.message);
          throw e;
        })
        .finally(() => {
          if (!abort.signal.aborted) setLoading(false);
        });
    }
    poll(load, {
      signal: abort.signal,
      interval: kind === "chats" ? 7000 : kind === "news" ? 15000 : 30000,
    });
    return () => {
      abort.abort();
    };
  }, [kind, key, cursor, refresh]);
  return {
    data,
    setData,
    loading,
    error,
    cursor,
    index: cursors.length,
    back: () => setPosition({ key, cursors: cursors.slice(0, -1) }),
    next: () => {
      if (data.next && data.next !== cursor)
        setPosition({ key, cursors: [...cursors, data.next] });
    },
  };
}
type RemoteResult = ReturnType<typeof useRemote>;
function Pages({
  result,
  label = "Страница",
}: {
  result: RemoteResult;
  label?: string;
}) {
  return (
    <nav className="bx-pages" aria-label={label}>
      <button
        className="icon-button"
        aria-label="Предыдущая страница"
        disabled={result.loading || result.index === 1}
        onClick={(e) => {
          result.back();
          e.currentTarget
            .closest("section")
            ?.scrollIntoView({ block: "start" });
        }}
      >
        <ChevronLeft size={17} />
      </button>
      <span>
        {label} {result.index}
      </span>
      <button
        className="icon-button"
        aria-label="Следующая страница"
        disabled={
          result.loading ||
          !!result.error ||
          !result.data.next ||
          result.data.next === result.cursor
        }
        onClick={(e) => {
          result.next();
          e.currentTarget
            .closest("section")
            ?.scrollIntoView({ block: "start" });
        }}
      >
        <ChevronRight size={17} />
      </button>
    </nav>
  );
}
function LoadState({
  result,
}: {
  result: Pick<RemoteResult, "loading" | "error">;
}) {
  return result.error ? (
    <div className="inline-warning" role="alert">
      {result.error}
    </div>
  ) : result.loading ? (
    <div className="bx-loading" role="status">
      <span className="loader" />
      Загружаем…
    </div>
  ) : null;
}
function useTerm(value: string) {
  const [term, setTerm] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setTerm(value.trim()), 300);
    return () => clearTimeout(t);
  }, [value]);
  return term;
}
function validDate(date?: string) {
  return !!date && !date.startsWith("0001");
}
function shortTime(date?: string) {
  return validDate(date)
    ? new Date(date!).toLocaleTimeString("ru-RU", {
        hour: "2-digit",
        minute: "2-digit",
      })
    : "";
}

export function BitrixWorkspace({
  focusId = "",
  focusRequest = 0,
}: {
  focusId?: string;
  focusRequest?: number;
}) {
  const { state } = useData();
  const [kind, setKind] = useState<Kind>("chats");
  const [refresh, setRefresh] = useState(0);
  const [chat, setChat] = useState<RemoteItem | null>(null);
  const [department, setDepartment] = useState("");
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [fileDrafts, setFileDrafts] = useState<Record<string, ChatFileDraft[]>>(
    {},
  );
  const [uncertain, setUncertain] = useState<Record<string, boolean>>({});
  const [dialogCache] = useState(() => new DialogCache());
  useEffect(() => {
    if (focusId) {
      setKind("chats");
      setChat({ id: focusId, title: "Диалог", kind: "chat" });
    }
  }, [focusId, focusRequest]);
  const tabs: [Kind, string, typeof MessageCircle][] = [
    ["chats", "Чаты", MessageCircle],
    ["news", "Новости", Newspaper],
    ["people", "Сотрудники", Users],
    ["departments", "Отделы", Network],
    ["files", "Файлы", Folder],
  ];
  function people(id: string) {
    setDepartment(id);
    setKind("people");
  }
  return (
    <>
      <div className="page-heading heading-flex">
        <div>
          <div className="eyebrow">КОМАНДА И ОБЩЕНИЕ</div>
          <h1>Битрикс24</h1>
          <p>Разговоры, люди и события вашей команды.</p>
        </div>
        <button className="button" onClick={() => setRefresh((n) => n + 1)}>
          <RefreshCw size={16} />
          Обновить
        </button>
      </div>
      <nav className="bx-tabs" aria-label="Разделы Битрикса">
        {tabs.map(([id, title, Icon]) => (
          <button
            key={id}
            aria-current={kind === id ? "page" : undefined}
            className={kind === id ? "active" : ""}
            onClick={() => setKind(id)}
          >
            <Icon size={17} />
            {title}
          </button>
        ))}
      </nav>
      {state.settings.demo ? (
        <Empty
          title="Рабочие данные в деморежиме скрыты"
          description="Отключите деморежим в настройках, чтобы увидеть Битрикс."
        />
      ) : (
        <>
          {kind === "chats" && (
            <Chats
              cache={dialogCache}
              refresh={refresh}
              selected={chat}
              select={setChat}
              drafts={drafts}
              fileDrafts={fileDrafts}
              setFiles={(key, update) =>
                setFileDrafts((v) => ({ ...v, [key]: update(v[key] || []) }))
              }
              setDraft={(id, text) => setDrafts((v) => ({ ...v, [id]: text }))}
              uncertain={uncertain}
              setUncertain={(id, value) =>
                setUncertain((v) => ({ ...v, [id]: value }))
              }
            />
          )}
          {kind === "news" && <News refresh={refresh} />}
          {kind === "people" && (
            <People
              refresh={refresh}
              department={department}
              setDepartment={setDepartment}
            />
          )}
          {kind === "departments" && (
            <Departments refresh={refresh} people={people} />
          )}
          {kind === "files" && <Files refresh={refresh} />}
        </>
      )}
    </>
  );
}
function Chats({
  cache,
  refresh,
  selected,
  select,
  drafts,
  fileDrafts,
  setFiles,
  setDraft,
  uncertain,
  setUncertain,
}: {
  cache: DialogCache;
  refresh: number;
  selected: RemoteItem | null;
  select: (v: RemoteItem | null) => void;
  drafts: Record<string, string>;
  fileDrafts: Record<string, ChatFileDraft[]>;
  setFiles: (key: string, update: FileDraftUpdate) => void;
  setDraft: (id: string, text: string) => void;
  uncertain: Record<string, boolean>;
  setUncertain: (id: string, v: boolean) => void;
}) {
  const result = useRemote("chats", {}, refresh);
  const { refresh: refreshState } = useData();
  const readOverrides = useRef(
    new Map<string, { messageId: string; unread: number }>(),
  );
  const account = result.data.accountId || "";
  const onRead = useCallback(
    (id: string, messageId: string, unread: number) => {
      readOverrides.current.set(account + ":" + id, { messageId, unread });
      result.setData((page) => ({
        ...page,
        items: page.items.map((c) =>
          c.id === id &&
          (!c.messageId || BigInt(c.messageId) <= BigInt(messageId))
            ? { ...c, unread, readMessageId: messageId }
            : c,
        ),
      }));
      void refreshState().catch(() => {});
    },
    [result.setData, refreshState, account],
  );
  const [filter, setFilter] = useState("");
  const shown = result.data.items
    .map((c) => {
      const read = readOverrides.current.get(account + ":" + c.id);
      return read &&
        c.messageId &&
        BigInt(c.messageId) <= BigInt(read.messageId) &&
        BigInt(c.readMessageId || "0") < BigInt(read.messageId)
        ? { ...c, unread: read.unread }
        : c;
    })
    .filter((x) =>
      (x.title + " " + plainRemoteText(x.text))
        .toLocaleLowerCase()
        .includes(filter.toLocaleLowerCase()),
    );
  const item = selected
    ? result.data.items.find((x) => x.id === selected.id) || selected
    : null;
  return (
    <section className={"bx-messenger " + (item ? "has-dialog" : "")}>
      <aside className="bx-chat-sidebar" aria-label="Список чатов">
        <div className="bx-list-heading">
          <h2>Сообщения</h2>
          <span>{result.data.items.length}</span>
        </div>
        <label className="bx-search">
          <Search size={16} />
          <input
            aria-label="Поиск чатов на странице"
            placeholder="Поиск на странице"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
        </label>
        <div className="bx-chat-list">
          <LoadState result={result} />
          {!result.loading &&
            shown.map((c) => (
              <button
                key={c.id}
                className={
                  "bx-chat-row " + (c.id === item?.id ? "selected" : "")
                }
                aria-pressed={c.id === item?.id}
                onClick={() => select(c)}
              >
                <Avatar name={c.title} src={c.avatarUrl} />
                <span className="bx-chat-summary">
                  <span>
                    <strong>{c.title}</strong>
                    <time>{shortTime(c.date)}</time>
                  </span>
                  <span>
                    <small>
                      {c.own ? "Вы: " : ""}
                      {plainRemoteText(c.text) ||
                        (c.attachments?.length
                          ? "📎 Вложение"
                          : c.messageId
                            ? "Сообщение"
                            : "Нет сообщений")}
                    </small>
                    {!!c.unread && <i className="bx-count">{c.unread}</i>}
                  </span>
                </span>
              </button>
            ))}
          {!result.loading && !result.error && !shown.length && (
            <Empty title="Чаты не найдены" />
          )}
        </div>
        <Pages result={result} label="Чаты" />
      </aside>
      {item ? (
        <ChatDialog
          key={account + ":" + item.id}
          account={account}
          cache={cache}
          item={item}
          refresh={refresh}
          onRead={onRead}
          back={() => select(null)}
          text={drafts[account + ":" + item.id] || ""}
          files={fileDrafts[account + ":" + item.id] || []}
          setFiles={(update) => setFiles(account + ":" + item.id, update)}
          setText={(text) => setDraft(account + ":" + item.id, text)}
          uncertain={!!uncertain[account + ":" + item.id]}
          setUncertain={(v) => setUncertain(account + ":" + item.id, v)}
        />
      ) : (
        <div className="bx-dialog-empty">
          <MessageCircle size={38} />
          <h2>Выберите диалог</h2>
          <p>
            Список чатов всегда рядом.
            <br />
            Откройте разговор, чтобы прочитать сообщения и ответить.
          </p>
        </div>
      )}
    </section>
  );
}
function ChatDialog({
  account,
  cache,
  item,
  refresh,
  onRead,
  back,
  text,
  files,
  setFiles,
  setText,
  uncertain,
  setUncertain,
}: {
  account: string;
  cache: DialogCache;
  item: RemoteItem;
  refresh: number;
  onRead: (id: string, messageId: string, unread: number) => void;
  back: () => void;
  text: string;
  files: ChatFileDraft[];
  setFiles: (update: FileDraftUpdate) => void;
  setText: (v: string) => void;
  uncertain: boolean;
  setUncertain: (v: boolean) => void;
}) {
  const { notify } = useData();
  const fileInput = useRef<HTMLInputElement>(null);
  const dragDepth = useRef(0);
  const [dragging, setDragging] = useState(false);
  const fileBusy = files.some((f) => f.state === "sending");
  const fileUncertain = files.some((f) => f.state === "uncertain");
  function attach(incoming: File[]) {
    if (busy || fileBusy || uncertain || fileUncertain) return;
    const result = addChatFiles(files, incoming);
    setFiles(() => result.items);
    setSendError(result.error);
  }
  const [data, setData] = useState<RemotePage>(
    () => cache.get(account, item.id, "") || { items: [] },
  );
  const [cursors, setCursors] = useState([""]);
  const cursor = cursors[cursors.length - 1];
  const [loading, setLoading] = useState(
      () => !cache.get(account, item.id, ""),
    ),
    [error, setError] = useState(""),
    [sendError, setSendError] = useState(""),
    [busy, setBusy] = useState(false),
    [reload, setReload] = useState(0);
  const scroll = useRef<HTMLDivElement>(null);
  const bottom = useRef(true);
  const newest = useRef("");
  const readThrough = useRef(item.readMessageId || "0");
  const reading = useRef(false);
  const [readError, setReadError] = useState("");
  const [readRetry, setReadRetry] = useState(0);
  const pendingBottom = useRef(true);
  const loadedCursor = useRef<string | undefined>(undefined);
  const [verified, setVerified] = useState(false);
  useLayoutEffect(() => {
    if (pendingBottom.current && scroll.current) {
      scroll.current.scrollTop = scroll.current.scrollHeight;
      pendingBottom.current = false;
    }
  }, [data.items, loading]);
  useEffect(() => {
    const a = new AbortController();
    let initial = loadedCursor.current !== cursor;
    const cached = cache.get(account, item.id, cursor);
    setData((old) => cached || (!initial ? old : { items: [] }));
    setLoading(!cached && initial);
    setVerified(false);
    setError("");
    pendingBottom.current = initial;
    async function load() {
      try {
        const value = await bitrixPage(
          "/workspace/bitrix?kind=messages&id=" +
            endpoint(item.id) +
            "&cursor=" +
            endpoint(cursor),
          a.signal,
        );
        if (a.signal.aborted) return;
        if (account && value.accountId !== account) {
          setData({ items: [] });
          throw new Error(
            "Учётная запись Битрикса изменилась. Обновите список чатов.",
          );
        }
        const ordered = sortMessages(value.items);
        const last = ordered.at(-1)?.id || "";
        const goBottom = initial || bottom.current;
        pendingBottom.current =
          goBottom && (initial || last !== newest.current);
        const page = { ...value, items: ordered };
        cache.put(account, item.id, cursor, page);
        loadedCursor.current = cursor;
        setData((old) =>
          JSON.stringify(old) === JSON.stringify(page) ? old : page,
        );
        setVerified(true);
        setError("");
        setLoading(false);
        newest.current = last;
        initial = false;
      } catch (e) {
        if (!a.signal.aborted) {
          setError((e as Error).message);
          setLoading(false);
        }
        throw e;
      }
    }
    poll(load, { signal: a.signal, interval: cursor ? 30000 : 4000 });
    return () => {
      a.abort();
    };
  }, [account, cache, item.id, cursor, refresh, reload]);
  useEffect(() => {
    const root = scroll.current;
    if (!root || loading || !verified || !data.accountId || error) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    async function acknowledge() {
      if (
        stopped ||
        reading.current ||
        document.visibilityState !== "visible" ||
        !document.hasFocus() ||
        document.querySelector('[aria-modal="true"]')
      )
        return;
      const bounds = root!.getBoundingClientRect();
      let messageId = readThrough.current;
      root!
        .querySelectorAll<HTMLElement>("[data-chat-message]")
        .forEach((el) => {
          const rect = el.getBoundingClientRect(),
            id = el.dataset.chatMessage!;
          if (
            /^[1-9][0-9]*$/.test(id) &&
            rect.width > 0 &&
            rect.height > 0 &&
            rect.right > 0 &&
            rect.left < window.innerWidth &&
            rect.top >= Math.max(0, bounds.top) &&
            rect.bottom <= Math.min(window.innerHeight, bounds.bottom) &&
            BigInt(id) > BigInt(messageId)
          )
            messageId = id;
        });
      if (messageId === readThrough.current) return;
      reading.current = true;
      try {
        const result = await api<{ messageId: string; unread: number }>(
          "/bitrix/chats/" + endpoint(item.id) + "/read",
          "PATCH",
          { messageId, accountId: data.accountId },
        );
        readThrough.current = messageId;
        onRead(item.id, messageId, result.unread);
        if (!stopped) setReadError("");
      } catch (e) {
        if (!stopped)
          setReadError(
            "Не удалось отметить сообщения прочитанными: " +
              (e as Error).message,
          );
      } finally {
        reading.current = false;
      }
    }
    function schedule() {
      clearTimeout(timer);
      timer = setTimeout(() => void acknowledge(), 250);
    }
    const observer = new IntersectionObserver(schedule, { root, threshold: 1 });
    root
      .querySelectorAll("[data-chat-message]")
      .forEach((el) => observer.observe(el));
    document.addEventListener("visibilitychange", schedule);
    document.addEventListener("focusin", schedule);
    window.addEventListener("focus", schedule);
    root.addEventListener("scroll", schedule);
    schedule();
    return () => {
      stopped = true;
      clearTimeout(timer);
      observer.disconnect();
      document.removeEventListener("visibilitychange", schedule);
      document.removeEventListener("focusin", schedule);
      window.removeEventListener("focus", schedule);
      root.removeEventListener("scroll", schedule);
    };
  }, [data, loading, verified, error, item.id, onRead, readRetry]);
  async function send() {
    if (
      (!text.trim() && !files.length) ||
      busy ||
      fileBusy ||
      uncertain ||
      fileUncertain
    )
      return;
    setBusy(true);
    setSendError("");
    try {
      if (files.length) {
        let caption = text;
        // Capture the dialog and ordered selection once: switching chats cannot
        // redirect an in-flight upload. Remove only confirmed sends from drafts.
        for (const file of files) {
          setFiles((v) =>
            v.map((f) =>
              f.id === file.id ? { ...f, state: "sending", progress: 0 } : f,
            ),
          );
          try {
            await uploadChatFile(item.id, account, file, caption, (progress) =>
              setFiles((v) =>
                v.map((f) => (f.id === file.id ? { ...f, progress } : f)),
              ),
            );
          } catch (e) {
            const ambiguous = !(e instanceof ChatUploadError) || e.uncertain;
            setFiles((v) =>
              v.map((f) =>
                f.id === file.id
                  ? { ...f, state: ambiguous ? "uncertain" : "pending" }
                  : f,
              ),
            );
            throw e;
          }
          setFiles((v) => v.filter((f) => f.id !== file.id));
          caption = "";
          setText("");
          setReload((n) => n + 1);
        }
      } else {
        await api("/bitrix/chats/" + endpoint(item.id) + "/messages", "POST", {
          text,
        });
      }
      setText("");
      setCursors([""]);
      setReload((n) => n + 1);
      notify("Сообщение отправлено");
    } catch (e) {
      setSendError((e as Error).message);
      setUncertain(!(e instanceof ChatUploadError) || e.uncertain);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div
      className="bx-dialog"
      onDragEnter={(e) => {
        if (e.dataTransfer.types.includes("Files")) {
          e.preventDefault();
          dragDepth.current++;
          if (!busy && !fileBusy) setDragging(true);
        }
      }}
      onDragOver={(e) => {
        if (e.dataTransfer.types.includes("Files")) {
          e.preventDefault();
          e.dataTransfer.dropEffect = busy || fileBusy ? "none" : "copy";
        }
      }}
      onDragLeave={(e) => {
        if (e.dataTransfer.types.includes("Files")) {
          e.preventDefault();
          dragDepth.current = Math.max(0, dragDepth.current - 1);
          if (!dragDepth.current) setDragging(false);
        }
      }}
      onDrop={(e) => {
        if (e.dataTransfer.types.includes("Files")) {
          e.preventDefault();
          dragDepth.current = 0;
          setDragging(false);
          attach(Array.from(e.dataTransfer.files));
        }
      }}
    >
      {dragging && (
        <div className="bx-drop-overlay">
          <Paperclip size={32} />
          <strong>Перетащите файлы сюда</strong>
          <span>Добавим к сообщению</span>
        </div>
      )}
      <header className="bx-dialog-header">
        <button
          className="icon-button bx-chat-back"
          onClick={back}
          aria-label="К списку чатов"
        >
          <ArrowLeft size={19} />
        </button>
        <Avatar name={item.title} src={item.avatarUrl} />
        <div>
          <h2>{item.title}</h2>
          <small>Битрикс24 · рабочий диалог</small>
        </div>
        {item.url && (
          <a
            className="icon-button"
            href={item.url}
            target="_blank"
            rel="noreferrer"
            aria-label="Открыть чат в Битриксе"
          >
            <ArrowUpRight size={18} />
          </a>
        )}
      </header>
      <div className="bx-history-pages">
        <button
          className="text-button"
          disabled={loading || !data.next}
          onClick={() => setCursors((v) => [...v, data.next!])}
        >
          Предыдущие сообщения
        </button>
        {cursors.length > 1 && (
          <button
            className="text-button"
            disabled={loading}
            onClick={() => setCursors((v) => v.slice(0, -1))}
          >
            Более новые <ChevronRight size={14} />
          </button>
        )}
      </div>
      {readError && (
        <div className="inline-warning" role="alert">
          {readError}
          <button
            className="text-button"
            onClick={() => setReadRetry((v) => v + 1)}
          >
            Повторить
          </button>
        </div>
      )}
      <div
        className="bx-message-scroll"
        ref={scroll}
        onScroll={() => {
          const el = scroll.current;
          if (el)
            bottom.current =
              el.scrollHeight - el.scrollTop - el.clientHeight < 90;
        }}
      >
        {error && (
          <div className="inline-warning" role="alert">
            {error}
            <button
              className="text-button"
              onClick={() => setReload((n) => n + 1)}
            >
              Повторить
            </button>
          </div>
        )}
        {loading ? (
          <div className="bx-loading" role="status">
            Загружаем сообщения…
          </div>
        ) : data.items.length ? (
          data.items.map((m, i) => (
            <Fragment key={m.id}>
              {validDate(m.date) &&
                (i === 0 ||
                  dayKey(data.items[i - 1].date || "") !== dayKey(m.date!)) && (
                  <div className="bx-message-day">{dayLabel(m.date!)}</div>
                )}
              <article
                className={
                  "bx-bubble " +
                  (m.own ? "own " : "") +
                  (m.system ? "system" : "")
                }
              >
                {!m.system && (
                  <header className="bx-message-author">
                    <Avatar name={m.title} src={m.avatarUrl} size="small" />
                    <strong>{m.own ? "Вы" : m.title}</strong>
                  </header>
                )}
                <BitrixText text={m.text} />
                <ChatCards item={m} />
                <ChatAttachments files={m.attachments} />
                <time data-chat-message={m.id}>{shortTime(m.date)}</time>
              </article>
            </Fragment>
          ))
        ) : (
          !error && <Empty title="В диалоге пока нет сообщений" />
        )}
      </div>
      <form
        className="bx-compose"
        onSubmit={(e) => {
          e.preventDefault();
          void send();
        }}
      >
        {sendError && (
          <div className="inline-warning" role="alert">
            {sendError}
          </div>
        )}
        {(uncertain || fileUncertain) && (
          <div className="inline-warning">
            Результат отправки не подтверждён. Проверьте чат перед повтором.
            <button
              className="text-button"
              type="button"
              onClick={() => {
                setUncertain(false);
                setFiles((v) =>
                  v.map((f) =>
                    f.state === "uncertain"
                      ? {
                          ...f,
                          id: crypto.randomUUID(),
                          state: "pending",
                          progress: 0,
                        }
                      : f,
                  ),
                );
                setSendError("");
                setReload((n) => n + 1);
              }}
            >
              Я проверил чат — разрешить повтор
            </button>
          </div>
        )}
        {!!files.length && (
          <ChatFileDrafts
            items={files}
            disabled={busy || fileBusy}
            remove={(id) => setFiles((v) => v.filter((f) => f.id !== id))}
          />
        )}
        <input
          ref={fileInput}
          className="bx-file-input"
          type="file"
          multiple
          aria-label="Выбрать вложения"
          onChange={(e) => {
            attach(Array.from(e.target.files || []));
            e.target.value = "";
          }}
        />
        <div>
          <button
            className="icon-button bx-attach-button"
            type="button"
            aria-label="Прикрепить файлы"
            title="Прикрепить файлы"
            disabled={busy || fileBusy || uncertain || fileUncertain}
            onClick={() => fileInput.current?.click()}
          >
            <Paperclip size={21} />
          </button>
          <textarea
            aria-label="Сообщение в Битрикс24"
            placeholder="Написать сообщение…"
            rows={2}
            maxLength={10000}
            disabled={busy || fileBusy}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onPaste={(e) => {
              if (e.clipboardData.files.length) {
                e.preventDefault();
                attach(Array.from(e.clipboardData.files));
              }
            }}
          />
          <button
            className="button primary"
            disabled={
              busy ||
              fileBusy ||
              (!text.trim() && !files.length) ||
              uncertain ||
              fileUncertain
            }
            type="submit"
          >
            <Send size={18} />
            <span>{busy || fileBusy ? "Отправляем…" : "Отправить"}</span>
          </button>
        </div>
      </form>
    </div>
  );
}
function News({ refresh }: { refresh: number }) {
  const result = useRemote("news", {}, refresh);
  return (
    <section className="bx-feed">
      <div className="bx-section-title">
        <div>
          <h2>Лента новостей</h2>
          <p>События компании · новые публикации сверху</p>
        </div>
      </div>
      <LoadState result={result} />
      {!result.loading &&
        result.data.items.map((item) => <NewsPost key={item.id} item={item} />)}
      {!result.loading && !result.error && !result.data.items.length && (
        <Empty
          title={
            result.data.next
              ? "На этой странице нет доступных публикаций"
              : "Публикаций больше нет"
          }
        />
      )}
      <Pages result={result} label="Лента" />
    </section>
  );
}
function NewsPost({ item }: { item: RemoteItem }) {
  const [expanded, setExpanded] = useState(false);
  const text = plainRemoteText(item.text);
  const long = text.length > 1000;
  return (
    <article className="bx-news-post">
      <header>
        <Avatar name={item.author || "Команда"} src={item.avatarUrl} />
        <div>
          <strong>{item.author || "Новости компании"}</strong>
          <time>
            {validDate(item.date)
              ? new Date(item.date!).toLocaleString("ru-RU", {
                  day: "numeric",
                  month: "long",
                  year: "numeric",
                  hour: "2-digit",
                  minute: "2-digit",
                })
              : ""}
          </time>
        </div>
        <Newspaper size={18} />
      </header>
      {item.title && <h2>{item.title}</h2>}
      {expanded || !long ? (
        <BitrixText text={item.text} className="bx-news-text" />
      ) : (
        <p className="bx-news-text">{text.slice(0, 1000) + "…"}</p>
      )}
      <footer>
        {long && (
          <button
            className="text-button"
            onClick={() => setExpanded((v) => !v)}
          >
            {expanded ? "Свернуть" : "Читать полностью"}
          </button>
        )}
        {item.url && (
          <a
            className="text-button"
            href={item.url}
            target="_blank"
            rel="noreferrer"
          >
            Открыть публикацию
            <ArrowUpRight size={15} />
          </a>
        )}
      </footer>
    </article>
  );
}
function People({
  refresh,
  department,
  setDepartment,
}: {
  refresh: number;
  department: string;
  setDepartment: (id: string) => void;
}) {
  const [status, setStatus] = useState("active");
  const [search, setSearch] = useState("");
  const term = useTerm(search);
  const result = useRemote(
    "people",
    { status, department, search: term },
    refresh,
  );
  const departments = useRemote("departments", {}, refresh);
  const names = new Map(departments.data.items.map((d) => [d.id, d.title]));
  const tags = [
    ["all", "Все"],
    ["active", "В компании"],
    ["dismissed", "Уволенные"],
    ["online", "Онлайн"],
  ];
  return (
    <section className="bx-directory">
      <div className="bx-section-title">
        <div>
          <h2>
            Сотрудники{" "}
            <span className="bx-total">{result.data.totalAll ?? "…"}</span>
          </h2>
          <p>Всего аккаунтов сотрудников, доступных вашей учётной записи</p>
        </div>
      </div>
      <div className="bx-directory-filters">
        <label className="bx-search">
          <Search size={17} />
          <input
            aria-label="Поиск сотрудников"
            placeholder="Имя или фамилия"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </label>
        <select
          aria-label="Отдел сотрудников"
          value={department}
          onChange={(e) => setDepartment(e.target.value)}
        >
          <option value="">Все отделы</option>
          {departments.data.items.map((d) => (
            <option key={d.id} value={d.id}>
              {d.parent && names.get(d.parent)
                ? names.get(d.parent) + " / "
                : ""}
              {d.title}
            </option>
          ))}
        </select>
      </div>
      <div className="bx-filter-tags" aria-label="Статус сотрудников">
        {tags.map(([id, label]) => (
          <button
            key={id}
            className={status === id ? "active" : ""}
            aria-pressed={status === id}
            onClick={() => setStatus(id)}
          >
            {label}
          </button>
        ))}
        {department && (
          <button
            className="bx-active-filter"
            onClick={() => setDepartment("")}
          >
            {names.get(department) || "Отдел"}
            <X size={13} />
          </button>
        )}
      </div>
      <div className="bx-found" role="status">
        {result.loading
          ? "Ищем сотрудников…"
          : result.error
            ? "Список сотрудников недоступен"
            : "Найдено: " + (result.data.total ?? result.data.items.length)}
        {!result.loading && !result.error && result.data.items.length > 0 && (
          <span>На странице: {result.data.items.length}</span>
        )}
      </div>
      {departments.error && (
        <p className="inline-warning">
          Список отделов недоступен. Поиск по имени и статусу продолжает
          работать.
        </p>
      )}
      {result.data.warning && (
        <p className="inline-warning">{result.data.warning}</p>
      )}
      <LoadState result={result} />
      {!result.loading && (
        <div className="bx-people-grid">
          {result.data.items.map((person) => (
            <article className="bx-person" key={person.id}>
              <div className="bx-person-top">
                <Avatar name={person.title} src={person.avatarUrl} />
                <span className={"bx-person-status " + person.status}>
                  {person.status === "dismissed" ? "Уволен" : "В компании"}
                </span>
              </div>
              <a href={person.url} target="_blank" rel="noreferrer">
                <h3>{person.title}</h3>
              </a>
              <p>{person.text || "Должность не указана"}</p>
              <div className="bx-person-departments">
                {person.departments?.map((id) => (
                  <button key={id} onClick={() => setDepartment(id)}>
                    {names.get(id) || "Отдел " + id}
                  </button>
                ))}
              </div>
              {person.online && (
                <small className="bx-online">Сейчас в сети</small>
              )}
            </article>
          ))}
        </div>
      )}
      {!result.loading && !result.error && !result.data.items.length && (
        <Empty
          title="Сотрудники не найдены"
          description="Измените имя, статус или выбранный отдел."
        />
      )}
      <Pages result={result} label="Сотрудники" />
    </section>
  );
}
function Departments({
  refresh,
  people,
}: {
  refresh: number;
  people: (id: string) => void;
}) {
  const result = useRemote("departments", {}, refresh);
  const [search, setSearch] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const tree = useMemo(
    () => departmentTree(result.data.items),
    [result.data.items],
  );
  const term = search.trim().toLocaleLowerCase();
  function matches(node: DepartmentNode): boolean {
    return (
      (node.item.title + " " + (node.item.headName || ""))
        .toLocaleLowerCase()
        .includes(term) || node.children.some(matches)
    );
  }
  function render(node: DepartmentNode, depth: number) {
    if (term && !matches(node)) return null;
    const d = node.item;
    const open = !!term || (expanded[d.id] ?? depth < 2);
    return (
      <li key={d.id}>
        <div className="bx-department-card">
          <div className="bx-department-title">
            <button
              className="icon-button"
              disabled={!node.children.length}
              aria-label={
                (open ? "Свернуть отдел " : "Развернуть отдел ") + d.title
              }
              aria-expanded={node.children.length ? open : undefined}
              onClick={() => setExpanded((v) => ({ ...v, [d.id]: !open }))}
            >
              <ChevronRight size={17} className={open ? "rotated" : ""} />
            </button>
            <Network size={19} />
            <h3>{d.title}</h3>
            {node.children.length > 0 && (
              <span>Подразделений: {node.children.length}</span>
            )}
          </div>
          <div className="bx-department-body">
            <div className="bx-head">
              <Avatar name={d.headName || "?"} src={d.headAvatarUrl} />
              <span>
                <small>Руководитель</small>
                {d.headName ? (
                  <a href={d.headUrl} target="_blank" rel="noreferrer">
                    {d.headName}
                    <ArrowUpRight size={13} />
                  </a>
                ) : (
                  <span className="muted">
                    {d.headId ? "Имя недоступно" : "Не назначен"}
                  </span>
                )}
              </span>
            </div>
            <button className="text-button" onClick={() => people(d.id)}>
              <Users size={15} />
              Сотрудники отдела
            </button>
          </div>
        </div>
        {node.children.length > 0 && open && (
          <ul>{node.children.map((child) => render(child, depth + 1))}</ul>
        )}
      </li>
    );
  }
  return (
    <section className="bx-structure">
      <div className="bx-section-title">
        <div>
          <h2>
            Структура компании{" "}
            <span className="bx-total">{result.data.total ?? "…"}</span>
          </h2>
          <p>Отделы, подчинённость и руководители из Битрикса</p>
        </div>
      </div>
      <div className="bx-structure-tools">
        <label className="bx-search">
          <Search size={17} />
          <input
            aria-label="Поиск отдела или руководителя"
            placeholder="Отдел или руководитель"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </label>
        <button
          className="button small"
          onClick={() =>
            setExpanded(
              Object.fromEntries(result.data.items.map((d) => [d.id, true])),
            )
          }
        >
          Развернуть всё
        </button>
        <button
          className="text-button"
          onClick={() =>
            setExpanded(
              Object.fromEntries(result.data.items.map((d) => [d.id, false])),
            )
          }
        >
          Свернуть всё
        </button>
      </div>
      <LoadState result={result} />
      {result.data.warning && (
        <p className="inline-warning">{result.data.warning}</p>
      )}
      {!result.loading && (
        <ul className="bx-org-tree">{tree.map((n) => render(n, 0))}</ul>
      )}
      {!result.loading && !result.error && !tree.some(matches) && (
        <Empty title="Отделы не найдены" />
      )}
    </section>
  );
}
function Files({ refresh }: { refresh: number }) {
  const [path, setPath] = useState<RemoteItem[]>([]);
  const parent = path.at(-1);
  const result = useRemote("files", { id: parent?.id || "" }, refresh);
  return (
    <section>
      <div className="bx-section-title">
        <h2>{parent?.title || "Файлы команды"}</h2>
        {parent && (
          <button
            className="text-button"
            onClick={() => setPath((v) => v.slice(0, -1))}
          >
            <ArrowLeft size={16} />
            Назад
          </button>
        )}
      </div>
      <LoadState result={result} />
      {!result.loading && (
        <div className="remote-grid">
          {result.data.items.map((item) =>
            item.kind === "folder" ? (
              <button
                key={item.id}
                className="remote-card"
                onClick={() => setPath((v) => [...v, item])}
              >
                <Folder size={20} />
                <strong>{item.title}</strong>
              </button>
            ) : (
              <article key={item.id} className="remote-card">
                {item.attachments?.length ? (
                  <ChatAttachments files={item.attachments} />
                ) : (
                  <strong>{item.title}</strong>
                )}
                {item.url ? (
                  <a
                    className="text-button"
                    href={item.url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Открыть в Битриксе
                    <ArrowUpRight size={15} />
                  </a>
                ) : (
                  !item.attachments?.length && (
                    <span className="muted">Откройте файл в Битриксе</span>
                  )
                )}
              </article>
            ),
          )}
        </div>
      )}
      {!result.loading && !result.error && !result.data.items.length && (
        <Empty title="В этой папке пока нет файлов" />
      )}
      <Pages result={result} label="Файлы" />
    </section>
  );
}

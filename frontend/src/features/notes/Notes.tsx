import { useEffect, useState } from "react";
import {
  Download,
  Plus,
  Pin,
  Search,
  Save,
  Trash2,
  StickyNote,
} from "lucide-react";
import { useData } from "../../app/store";
import { unchanged } from "../../shared/live";
import { api, download, endpoint } from "../../shared/api";
import { Empty } from "../../shared/ui";
import type { Note } from "../../shared/types";
export function Notes({ focusId }: { focusId?: string }) {
  const { state, act } = useData();
  const [search, setSearch] = useState("");
  const [editing, setEditing] = useState<Note | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [pending, setPending] = useState<Note | null>(null);
  const latest = state.notes.find((n) => n.id === editing?.id);
  useEffect(() => {
    if (!dirty && !busy && editing?.id)
      setEditing((current) => unchanged(current, latest || null));
  }, [latest, dirty, busy, editing?.id]);
  useEffect(() => {
    if (focusId) {
      const n = state.notes.find((n) => n.id === focusId);
      if (n) {
        setEditing(n);
        setDirty(false);
      }
    }
  }, [focusId]);
  const notes = state.notes
    .filter((n) =>
      `${n.title} ${n.body} ${n.tag}`
        .toLowerCase()
        .includes(search.toLowerCase()),
    )
    .sort(
      (a, b) =>
        Number(b.pinned) - Number(a.pinned) ||
        b.updatedAt.localeCompare(a.updatedAt),
    );
  function choose(n: Note) {
    setConfirm(false);
    if (dirty) {
      setPending(n);
      return;
    }
    setEditing(n);
  }
  function create() {
    choose({
      id: "",
      title: "",
      body: "",
      tag: "Личное",
      pinned: false,
      updatedAt: new Date().toISOString(),
      demo: state.settings.demo,
    });
  }
  function update(p: Partial<Note>) {
    if (editing) {
      setEditing({ ...editing, ...p });
      setDirty(true);
    }
  }
  async function save() {
    if (!editing) return;
    setBusy(true);
    try {
      const n = await act(
        () =>
          api<Note>(
            "/notes" + (editing.id ? "/" + endpoint(editing.id) : ""),
            editing.id ? "PUT" : "POST",
            editing,
          ),
        "Заметка сохранена",
      );
      setEditing(n);
      setDirty(false);
    } catch {
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (!editing) return;
    setBusy(true);
    try {
      await act(
        () => api("/notes/" + endpoint(editing.id), "DELETE"),
        "Заметка удалена",
      );
      setEditing(null);
      setDirty(false);
      setConfirm(false);
    } catch {
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <div className="page-heading heading-flex">
        <div>
          <div className="eyebrow">ВАША БАЗА КОНТЕКСТА</div>
          <h1>Заметки</h1>
          <p>Договорённости, идеи и мысли, к которым стоит вернуться.</p>
        </div>
        <button className="button primary" onClick={create}>
          <Plus size={17} />
          Новая заметка
        </button>
      </div>
      <div className="notes-layout">
        <aside className="notes-list">
          <div className="search-field">
            <Search size={16} />
            <input
              placeholder="Найти в заметках…"
              aria-label="Поиск заметок"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          {notes.map((n) => (
            <button
              className={"note-item " + (editing?.id === n.id ? "active" : "")}
              key={n.id}
              onClick={() => choose(n)}
            >
              <div>
                <span className="tag">{n.tag || "Без метки"}</span>
                {n.pinned && <Pin size={14} />}
              </div>
              <h3>{n.title}</h3>
              <p>{n.body}</p>
              <time>
                {new Date(n.updatedAt).toLocaleDateString("ru-RU", {
                  day: "numeric",
                  month: "long",
                })}
              </time>
            </button>
          ))}
          {!notes.length && <Empty title="Здесь будет ваш контекст" />}
        </aside>
        <section className="note-editor panel">
          {editing ? (
            <>
              <div className="note-editor-toolbar">
                <input
                  className="tag-input"
                  aria-label="Категория заметки"
                  value={editing.tag}
                  onChange={(e) => update({ tag: e.target.value })}
                />
                <span className="spacer" />
                <span className="save-state">
                  {dirty ? "Есть несохранённые изменения" : "Сохранено"}
                </span>
                <button
                  className={"icon-button " + (editing.pinned ? "active" : "")}
                  title="Закрепить"
                  aria-label="Закрепить заметку"
                  onClick={() => update({ pinned: !editing.pinned })}
                >
                  <Pin size={17} />
                </button>
                <button
                  className="icon-button"
                  title="Экспорт Markdown"
                  onClick={() =>
                    download(
                      "note.md",
                      "# " + editing.title + "\n\n" + editing.body,
                    )
                  }
                >
                  <Download size={17} />
                </button>
                {editing.id && (
                  <button
                    className="icon-button"
                    title="Удалить заметку"
                    onClick={() => setConfirm(true)}
                  >
                    <Trash2 size={17} />
                  </button>
                )}
              </div>
              {confirm && (
                <div className="inline-warning">
                  Удалить эту заметку?
                  <button
                    className="text-button"
                    disabled={busy}
                    onClick={() => void remove()}
                  >
                    Да, удалить
                  </button>
                  <button
                    className="text-button"
                    onClick={() => setConfirm(false)}
                  >
                    Отмена
                  </button>
                </div>
              )}
              {pending && (
                <div className="inline-warning">
                  Есть несохранённые изменения.
                  <button
                    className="text-button"
                    onClick={() => {
                      setEditing(pending);
                      setPending(null);
                      setDirty(false);
                    }}
                  >
                    Отбросить и перейти
                  </button>
                  <button
                    className="text-button"
                    onClick={() => setPending(null)}
                  >
                    Остаться
                  </button>
                </div>
              )}
              <input
                className="note-title-input"
                aria-label="Заголовок заметки"
                placeholder="Название заметки"
                maxLength={200}
                value={editing.title}
                onChange={(e) => update({ title: e.target.value })}
              />
              <textarea
                className="note-body-input"
                aria-label="Текст заметки"
                placeholder="Начните писать… Поддерживается обычный текст и Markdown для экспорта."
                value={editing.body}
                onChange={(e) => update({ body: e.target.value })}
              />
              <div className="note-editor-footer">
                <span>{editing.body.length} символов · текст / Markdown</span>
                <button
                  className="button primary"
                  onClick={() => void save()}
                  disabled={busy || !editing.title.trim()}
                >
                  <Save size={16} />
                  {busy ? "Сохранение…" : "Сохранить"}
                </button>
              </div>
            </>
          ) : (
            <div className="notes-welcome">
              <StickyNote size={38} />
              <h2>Место, где мысли становятся яснее</h2>
              <p>
                Откройте заметку слева или начните новую.
                <br />
                Например, подготовку к 1:1 или итоги ретроспективы.
              </p>
              <button className="button" onClick={create}>
                <Plus size={17} />
                Создать заметку
              </button>
            </div>
          )}
        </section>
      </div>
    </>
  );
}

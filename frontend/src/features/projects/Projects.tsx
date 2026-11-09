import { useEffect, useRef, useState } from "react";
import {
  Plus,
  Upload,
  FolderKanban,
  Folder,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { api } from "../../shared/api";
import { poll, unchanged } from "../../shared/live";
import { Modal, Empty } from "../../shared/ui";
import { useData } from "../../app/store";
import { ProjectImport } from "./ProjectImport";
import { ProjectSheetSync } from "./ProjectSheetSync";
import { ProjectDetail } from "./ProjectDetail";
import "./projects.css";
export type Project = {
  sourceVisibility?: "row" | "sheet";
  remoteVersion?: Project;
  group?: string;
  historyCount?: number;
  history?: {
    key: string;
    label: string;
    text: string;
    date?: string;
    note?: string;
    url?: string;
  }[];
  fields?: { key: string; name: string; value: string; note?: string }[];
  syncState?: string;
  syncMessage?: string;
  sheet?: { spreadsheetId: string; sheetName: string };
  id: string;
  name: string;
  status: string;
  state: string;
  owner: string;
  description: string;
  startDate: string;
  dueDate: string;
  url: string;
  source: string;
  version: number;
  updatedAt?: string;
};
const blank: Project = {
  id: "",
  name: "",
  status: "В работе",
  state: "active",
  owner: "",
  description: "",
  startDate: "",
  dueDate: "",
  url: "",
  source: "",
  version: 0,
};
const states = [
  ["active", "Активные"],
  ["completed", "Завершённые"],
  ["archived", "Архив"],
  ["all", "Все"],
];
export function Projects() {
  const { state, notify } = useData();
  const loadedQuery = useRef("");
  const [folder, setFolder] = useState<string | null>(null);
  const [includeHidden, setIncludeHidden] = useState(false);
  const [detail, setDetail] = useState("");
  const [search, setSearch] = useState(""),
    [term, setTerm] = useState(""),
    [filter, setFilter] = useState("active"),
    [page, setPage] = useState(1),
    [reload, setReload] = useState(0);
  const [data, setData] = useState<{
    items: Project[];
    total: number;
    counts: Record<string, number>;
    groups?: string[];
    hiddenTotal?: number;
  } | null>(null);
  const [loading, setLoading] = useState(true),
    [error, setError] = useState("");
  const [editing, setEditing] = useState<Project | null>(null),
    [importing, setImporting] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => {
      setTerm(search.trim());
      setPage(1);
    }, 250);
    return () => clearTimeout(t);
  }, [search]);
  useEffect(() => {
    setDetail("");
  }, [folder, filter, term, page, includeHidden, state.settings.demo]);
  useEffect(() => {
    const a = new AbortController();
    const query = JSON.stringify([
      filter,
      page,
      term,
      folder,
      includeHidden,
      state.settings.demo,
    ]);
    setLoading(loadedQuery.current !== query);
    setError("");
    const params = new URLSearchParams({
      state: filter,
      page: String(page),
      limit: "25",
      search: term,
      includeHidden: String(includeHidden),
    });
    if (folder === "") params.set("unfiled", "true");
    else if (folder) params.set("folder", folder);
    poll(
      () =>
        api<typeof data>(`/projects?${params}`, "GET", undefined, a.signal)
          .then((v) => {
            if (!a.signal.aborted) {
              setData((old) => unchanged(old, v));
              setError("");
              loadedQuery.current = query;
            }
          })
          .catch((e) => {
            if (!a.signal.aborted) setError(e.message);
            throw e;
          })
          .finally(() => {
            if (!a.signal.aborted) setLoading(false);
          }),
      { signal: a.signal, interval: 10000 },
    );
    return () => a.abort();
  }, [filter, page, term, folder, includeHidden, reload, state.settings.demo]);
  const saved = () => {
    setEditing(null);
    setImporting(false);
    setReload((v) => v + 1);
    notify("Проекты сохранены");
  };
  const folders = [
    ...new Set(
      (data?.groups || []).map((g) => g.split(" / ")[0]).filter(Boolean),
    ),
  ];
  const current = data?.items.find((p) => p.id === detail);
  return (
    <>
      <div className="page-heading heading-flex">
        <div>
          <div className="eyebrow">Рабочие направления</div>
          <h1>Проекты</h1>
          <p>Папки, текущие статусы и история каждого проекта.</p>
        </div>
        <div className="project-actions">
          <button className="button" onClick={() => setImporting(true)}>
            <Upload size={16} />
            Импорт таблицы
          </button>
          <button
            className="button primary"
            onClick={() => setEditing({ ...blank, group: folder || "" })}
          >
            <Plus size={16} />
            Создать проект
          </button>
        </div>
      </div>
      {!state.settings.demo && (
        <ProjectSheetSync changed={() => setReload((v) => v + 1)} />
      )}
      <div className={"projects-workspace" + (detail ? " has-selection" : "")}>
        <aside className="projects-sidebar" aria-label="Список проектов">
          <div className="projects-sidebar-controls">
            <label className="project-folder-picker">
              <Folder size={16} />
              <select
                aria-label="Папка проектов"
                value={folder === null ? "all" : "folder:" + folder}
                onChange={(e) => {
                  setFolder(
                    e.target.value === "all" ? null : e.target.value.slice(7),
                  );
                  setPage(1);
                }}
              >
                <option value="all">Все папки</option>
                <option value="folder:">Без папки</option>
                {folders.map((g) => (
                  <option key={g} value={"folder:" + g}>
                    {g}
                  </option>
                ))}
              </select>
            </label>
            <input
              aria-label="Поиск проектов"
              placeholder="Найти проект или ответственного"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <div className="project-state-tabs" aria-label="Состояние проектов">
              {states.map(([key, title]) => (
                <button
                  key={key}
                  className={filter === key ? "active" : ""}
                  aria-pressed={filter === key}
                  onClick={() => {
                    setFilter(key);
                    setPage(1);
                  }}
                >
                  {title}
                  {key !== "all" && <span>{data?.counts[key] || 0}</span>}
                </button>
              ))}
            </div>
            {!!data?.hiddenTotal && (
              <label className="project-hidden-toggle">
                <input
                  type="checkbox"
                  checked={includeHidden}
                  onChange={(e) => {
                    setIncludeHidden(e.target.checked);
                    setFolder(null);
                    setPage(1);
                  }}
                />
                Показать скрытые в таблице ({data.hiddenTotal})
              </label>
            )}
          </div>
          <div className="project-list" aria-busy={loading}>
            {error ? (
              <div className="inline-warning" role="alert">
                {error}
                <button
                  className="text-button"
                  onClick={() => setReload((v) => v + 1)}
                >
                  Повторить
                </button>
              </div>
            ) : loading ? (
              <p className="project-list-message" role="status">
                Загружаем проекты…
              </p>
            ) : data?.items.length ? (
              data.items.map((p) => (
                <button
                  key={p.id}
                  className={
                    "project-list-item" + (detail === p.id ? " selected" : "")
                  }
                  aria-pressed={detail === p.id}
                  aria-label={"Открыть проект " + p.name}
                  onClick={() => setDetail(p.id)}
                >
                  <span className="project-list-name">{p.name}</span>
                  {p.sourceVisibility && (
                    <small className="project-list-meta">
                      Скрыт в исходной таблице
                    </small>
                  )}
                  <span className="project-list-status">{p.status}</span>
                  <span className="project-list-meta">
                    {p.group || "Без папки"}
                    {p.owner ? " · " + p.owner : ""}
                  </span>
                  {p.syncState === "conflict" && (
                    <span className="project-list-sync">
                      Изменения требуют проверки
                    </span>
                  )}
                  {p.syncState === "pending" && (
                    <span className="project-list-sync">
                      Ожидает отправки в таблицу
                    </span>
                  )}
                </button>
              ))
            ) : (
              <p className="project-list-message">
                {term
                  ? "Проекты не найдены"
                  : "В этом разделе пока нет проектов"}
              </p>
            )}
          </div>
          <nav className="project-pagination" aria-label="Страницы проектов">
            <span>Всего: {data?.total || 0}</span>
            <button
              className="icon-button"
              aria-label="Предыдущая страница проектов"
              disabled={loading || page === 1}
              onClick={() => setPage((v) => v - 1)}
            >
              <ChevronLeft size={18} />
            </button>
            <span>
              {page} / {Math.max(1, Math.ceil((data?.total || 0) / 25))}
            </span>
            <button
              className="icon-button"
              aria-label="Следующая страница проектов"
              disabled={loading || page * 25 >= (data?.total || 0)}
              onClick={() => setPage((v) => v + 1)}
            >
              <ChevronRight size={18} />
            </button>
          </nav>
        </aside>
        <section
          className="project-detail-pane"
          aria-label="Информация о проекте"
        >
          {detail ? (
            <ProjectDetail
              key={detail}
              id={detail}
              revision={`${reload}:${current?.version || 0}`}
              close={() => setDetail("")}
              edit={setEditing}
              changed={() => setReload((v) => v + 1)}
            />
          ) : (
            <div className="project-empty">
              <FolderKanban size={44} />
              <Empty
                title="Выберите проект слева"
                description="Здесь появятся его статус, ответственные, сроки, все поля и история изменений."
              />
            </div>
          )}
        </section>
      </div>
      {editing && (
        <ProjectEditor
          project={editing}
          folders={data?.groups || []}
          close={() => setEditing(null)}
          saved={saved}
        />
      )}
      {importing && (
        <ProjectImport close={() => setImporting(false)} saved={saved} />
      )}
    </>
  );
}
function ProjectEditor({
  project,
  folders,
  close,
  saved,
}: {
  project: Project;
  folders: string[];
  close: () => void;
  saved: () => void;
}) {
  const { state } = useData();
  const [p, setP] = useState(project),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [loadingDetail, setLoadingDetail] = useState(
    !!project.sheet && !project.fields,
  );
  useEffect(() => {
    if (!project.sheet || project.fields) return;
    const a = new AbortController();
    void api<Project>(
      "/projects/" + encodeURIComponent(project.id),
      "GET",
      undefined,
      a.signal,
    )
      .then((v) => {
        if (!a.signal.aborted) setP(v);
      })
      .catch((e) => {
        if (!a.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!a.signal.aborted) setLoadingDetail(false);
      });
    return () => a.abort();
  }, [project.id]);
  const field = (key: keyof Project, value: string) =>
    setP((v) => ({ ...v, [key]: value }));
  if (loadingDetail)
    return (
      <Modal title="Редактирование проекта" onClose={close}>
        <p role="status">Загружаем поля проекта…</p>
      </Modal>
    );
  return (
    <Modal
      title={p.id ? "Редактирование проекта" : "Новый проект"}
      onClose={close}
    >
      <form
        className="project-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (busy) return;
          setBusy(true);
          setError("");
          void api("/projects", "POST", p)
            .then(saved)
            .catch((e) => setError(e.message))
            .finally(() => setBusy(false));
        }}
      >
        <label>
          Название
          <input
            required
            maxLength={160}
            value={p.name}
            onChange={(e) => field("name", e.target.value)}
          />
        </label>
        <label>
          Папка
          <input
            aria-label="Папка проекта"
            list="project-folder-options"
            maxLength={300}
            value={p.group || ""}
            disabled={!!p.sheet}
            onChange={(e) => field("group", e.target.value)}
            placeholder="Без папки"
          />
          <datalist id="project-folder-options">
            {folders.map((f) => (
              <option key={f} value={f} />
            ))}
          </datalist>
          {p.sheet && (
            <small>Папка определяется листом подключённой таблицы.</small>
          )}
        </label>
        <div className="project-form-row">
          <label>
            Статус
            <textarea
              aria-label="Статус"
              required
              rows={4}
              maxLength={20000}
              value={p.status}
              onChange={(e) => field("status", e.target.value)}
            />
          </label>
          <label>
            Раздел
            <select
              value={p.state}
              onChange={(e) => field("state", e.target.value)}
            >
              {states.slice(0, 3).map(([v, l]) => (
                <option key={v} value={v}>
                  {l}
                </option>
              ))}
            </select>
          </label>
        </div>
        {!p.id && !state.settings.demo && (
          <label className="project-sync-choice">
            <input
              type="checkbox"
              checked={p.syncState === "new-sheet"}
              onChange={(e) =>
                field("syncState", e.target.checked ? "new-sheet" : "")
              }
            />
            Добавить также в подключённую Google Таблицу
          </label>
        )}
        <label>
          Ответственный
          <input
            maxLength={100}
            value={p.owner}
            onChange={(e) => field("owner", e.target.value)}
          />
        </label>
        <div className="project-form-row">
          <label>
            Начало
            <input
              type="date"
              value={p.startDate}
              onChange={(e) => field("startDate", e.target.value)}
            />
          </label>
          <label>
            Срок
            <input
              type="date"
              value={p.dueDate}
              onChange={(e) => field("dueDate", e.target.value)}
            />
          </label>
        </div>
        <label>
          Описание
          <textarea
            rows={4}
            maxLength={7000}
            value={p.description}
            onChange={(e) => field("description", e.target.value)}
          />
        </label>
        <label>
          Ссылка
          <input
            type="url"
            value={p.url}
            onChange={(e) => field("url", e.target.value)}
          />
        </label>
        {p.fields?.map((f, i) =>
          /^c[1-9][0-9]*$/.test(f.key) && !/ответствен/i.test(f.name) ? (
            <label key={f.key}>
              {f.name}
              <textarea
                value={f.value}
                maxLength={20000}
                rows={2}
                onChange={(e) =>
                  setP((v) => ({
                    ...v,
                    fields: v.fields?.map((old, j) =>
                      j === i ? { ...old, value: e.target.value } : old,
                    ),
                  }))
                }
              />
            </label>
          ) : null,
        )}
        {error && (
          <p className="inline-warning" role="alert">
            {error}
          </p>
        )}
        <footer>
          <button
            type="button"
            className="button"
            disabled={busy}
            onClick={close}
          >
            Отмена
          </button>
          <button className="button primary" disabled={busy}>
            {busy ? "Сохранение…" : "Сохранить проект"}
          </button>
        </footer>
      </form>
    </Modal>
  );
}

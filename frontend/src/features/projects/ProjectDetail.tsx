import { useEffect, useState } from "react";
import { api, endpoint, dayLabel } from "../../shared/api";
import { poll, unchanged } from "../../shared/live";
import { ArrowLeft } from "lucide-react";
import type { Project } from "./Projects";
export function ProjectDetail({
  id,
  revision,
  close,
  edit,
  changed,
}: {
  id: string;
  revision: string;
  close: () => void;
  edit: (p: Project) => void;
  changed: () => void;
}) {
  const [project, setProject] = useState<Project | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [limit, setLimit] = useState(20);
  useEffect(() => {
    const a = new AbortController();
    setError("");
    poll(
      () =>
        api<Project>("/projects/" + endpoint(id), "GET", undefined, a.signal)
          .then((v) => {
            if (!a.signal.aborted) {
              setProject((old) => unchanged(old, v));
              setError("");
            }
          })
          .catch((e) => {
            if (!a.signal.aborted) setError(e.message);
            throw e;
          }),
      { signal: a.signal, interval: 10000 },
    );
    return () => a.abort();
  }, [id, revision]);
  async function resolve(useRemote: boolean) {
    if (!project) return;
    setBusy(true);
    setError("");
    try {
      await api(`/projects/${endpoint(id)}/resolve`, "POST", {
        version: project.version,
        useRemote,
      });
      changed();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="project-detail-view">
      <header className="project-detail-heading">
        <button
          className="icon-button"
          aria-label="Назад к списку проектов"
          onClick={close}
        >
          <ArrowLeft size={20} />
        </button>
        <div>
          <small>{project?.group || "Без папки"}</small>
          <h2>{project?.name || "Проект"}</h2>
        </div>
      </header>
      {error && (
        <p className="inline-warning" role="alert">
          {error}
        </p>
      )}
      {!project ? (
        !error && <p role="status">Загружаем проект…</p>
      ) : (
        <div className="project-detail">
          {project.sourceVisibility && (
            <p className="inline-warning">
              {project.sourceVisibility === "sheet"
                ? "Лист этого проекта скрыт в исходной таблице."
                : "Строка этого проекта скрыта в исходной таблице."}{" "}
              История и ваши изменения сохранены.
            </p>
          )}
          <dl className="project-source-fields project-core-fields">
            <div>
              <dt>Состояние</dt>
              <dd>
                {{
                  active: "Активный",
                  completed: "Завершён",
                  archived: "В архиве",
                }[project.state] || project.state}
              </dd>
            </div>
            <div>
              <dt>Ответственный</dt>
              <dd>{project.owner || "Не назначен"}</dd>
            </div>
            <div>
              <dt>Начало</dt>
              <dd>
                {project.startDate ? dayLabel(project.startDate) : "Не указано"}
              </dd>
            </div>
            <div>
              <dt>Срок</dt>
              <dd>
                {project.dueDate ? dayLabel(project.dueDate) : "Не указан"}
              </dd>
            </div>
            <div>
              <dt>Источник</dt>
              <dd>{project.source || "Создан в LeadHelper"}</dd>
            </div>
            {project.url && (
              <div>
                <dt>Ссылка</dt>
                <dd>
                  <a href={project.url} target="_blank" rel="noreferrer">
                    {project.url}
                  </a>
                </dd>
              </div>
            )}
          </dl>
          {project.syncState === "pending" && (
            <p className="project-list-sync">
              Изменения ожидают отправки в Google Таблицу.
            </p>
          )}
          <h3>Текущий статус</h3>
          <p>{project.status}</p>
          {project.description && (
            <>
              <h3>Описание</h3>
              <p>{project.description}</p>
            </>
          )}
          {project.remoteVersion && (
            <table className="project-conflict-table">
              <thead>
                <tr>
                  <th>Поле</th>
                  <th>В сервисе</th>
                  <th>В таблице</th>
                </tr>
              </thead>
              <tbody>
                {(
                  [
                    ["name", "Название"],
                    ["status", "Статус"],
                    ["owner", "Ответственный"],
                    ["state", "Раздел"],
                    ["description", "Описание"],
                    ["startDate", "Начало"],
                    ["dueDate", "Срок"],
                    ["url", "Ссылка"],
                  ] as const
                )
                  .filter(
                    ([key]) => project[key] !== project.remoteVersion![key],
                  )
                  .map(([key, label]) => (
                    <tr key={key}>
                      <th>{label}</th>
                      <td>{project[key]}</td>
                      <td>{project.remoteVersion![key]}</td>
                    </tr>
                  ))}
              </tbody>
            </table>
          )}
          {project.remoteVersion && (
            <dl className="project-source-fields">
              {project.fields
                ?.filter(
                  (f) =>
                    f.value !==
                    project.remoteVersion?.fields?.find((v) => v.key === f.key)
                      ?.value,
                )
                .map((f) => (
                  <div key={f.key}>
                    <dt>{f.name}</dt>
                    <dd>
                      <b>В сервисе: </b>
                      {f.value}
                      <br />
                      <b>В таблице: </b>
                      {project.remoteVersion?.fields?.find(
                        (v) => v.key === f.key,
                      )?.value || "Поле отсутствует"}
                    </dd>
                  </div>
                ))}
            </dl>
          )}
          {project.syncState === "conflict" ? (
            <div className="inline-warning">
              <p>{project.syncMessage}</p>
              <div className="project-actions">
                <button
                  className="button"
                  disabled={busy}
                  onClick={() => void resolve(true)}
                >
                  Принять версию таблицы
                </button>
                <button
                  className="button"
                  disabled={busy}
                  onClick={() => void resolve(false)}
                >
                  Отправить мою версию
                </button>
              </div>
            </div>
          ) : (
            <button className="button" onClick={() => edit(project)}>
              Редактировать проект
            </button>
          )}
          {!!project.fields?.length && (
            <dl className="project-source-fields">
              {project.fields.map((f) => (
                <div key={f.key}>
                  <dt>{f.name}</dt>
                  <dd>
                    {f.value}
                    {f.note && <small>{f.note}</small>}
                  </dd>
                </div>
              ))}
            </dl>
          )}
          <h3>История статусов · {project.history?.length || 0}</h3>
          {[...(project.history || [])]
            .reverse()
            .slice(0, limit)
            .map((h) => (
              <article className="project-history-entry" key={h.key}>
                <strong>{h.label}</strong>
                <p>{h.text}</p>
                {h.note && <small>{h.note}</small>}
                {h.url && /^https?:\/\//i.test(h.url) && (
                  <a href={h.url} target="_blank" rel="noreferrer">
                    Связанный документ ↗
                  </a>
                )}
              </article>
            ))}
          {(project.history?.length || 0) > limit && (
            <button className="button" onClick={() => setLimit((n) => n + 20)}>
              Ещё 20 записей
            </button>
          )}
        </div>
      )}
    </div>
  );
}

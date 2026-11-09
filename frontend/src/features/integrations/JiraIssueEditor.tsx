import { useEffect, useRef, useState, type FormEvent } from "react";
import { Plus, X, ExternalLink } from "lucide-react";
import { useData } from "../../app/store";
import { api, APIError, endpoint } from "../../shared/api";
import { Modal } from "../../shared/ui";
import { useDebounced } from "../../shared/taskPages";
import type { Member, Task, JiraUser } from "../../shared/types";
import "./jira-create.css";
import { JiraDescription } from "./JiraDescription";

type Option = { id: string; name: string; key?: string; subtask?: boolean };
type Value = Record<string, unknown>;
type Field = {
  id: string;
  name: string;
  required: boolean;
  schema: { type?: string; items?: string; custom?: string };
  allowedValues?: Value[];
  defaultValue?: unknown;
};
type Meta = { fields: Field[]; cloud: boolean; url: string };
type Created = { task: Task; warnings: string[] };
type Upload = { name: string; data: string };
const labelOf = (o: Value) =>
  String(o.name ?? o.value ?? o.displayName ?? o.id ?? "");
const optionValue = (o: Value) =>
  o.id
    ? { id: String(o.id) }
    : o.accountId
      ? { accountId: o.accountId }
      : o.name
        ? { name: o.name }
        : { value: o.value };
const optionKey = (o: Value) =>
  String(o.id ?? o.accountId ?? o.name ?? o.value ?? "");
const object = (v: unknown): Value =>
  v && typeof v === "object" && !Array.isArray(v) ? (v as Value) : {};
const formatHours = (v: unknown) => String(v ?? "");

export function JiraIssueEditor({
  member,
  onClose,
  onCreated,
}: {
  member?: Member;
  onClose: () => void;
  onCreated?: (task: Task) => void;
}) {
  const { refresh } = useData();
  const [projects, setProjects] = useState<Option[]>([]),
    [types, setTypes] = useState<Option[]>([]);
  const [project, setProject] = useState(""),
    [type, setType] = useState("");
  const [meta, setMeta] = useState<Meta | null>(null),
    [values, setValues] = useState<Value>({});
  const [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false);
  const [files, setFiles] = useState<Upload[]>([]),
    [fileError, setFileError] = useState("");
  const [reading, setReading] = useState(false),
    [created, setCreated] = useState<Created | null>(null);
  const [uncertain, setUncertain] = useState(false),
    [retry, setRetry] = useState(0);
  const requestId = useRef(crypto.randomUUID());
  const submitted = useRef<unknown>(null);
  useEffect(() => {
    const c = new AbortController();
    setLoading(true);
    setError("");
    void api<Option[]>("/jira/create/projects", "GET", undefined, c.signal)
      .then(setProjects)
      .catch((e) => {
        if (!c.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [retry]);
  useEffect(() => {
    setType("");
    setTypes([]);
    setMeta(null);
    setValues({});
    if (!project) return;
    const c = new AbortController();
    setLoading(true);
    setError("");
    void api<Option[]>(
      `/jira/create/types?project=${endpoint(project)}`,
      "GET",
      undefined,
      c.signal,
    )
      .then((v) => {
        setTypes(v);
        if (!v.length)
          setError(
            "Нет доступных типов задач. Проверьте право Create Issues в этом проекте.",
          );
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [project, retry]);
  useEffect(() => {
    setMeta(null);
    setValues({});
    if (!project || !type) return;
    const c = new AbortController();
    setLoading(true);
    setError("");
    void api<Meta>(
      `/jira/create/fields?project=${endpoint(project)}&type=${endpoint(type)}`,
      "GET",
      undefined,
      c.signal,
    )
      .then((v) => {
        setMeta(v);
        const initial: Value = {};
        for (const f of v.fields)
          if (f.defaultValue !== undefined && f.defaultValue !== null)
            initial[f.id] = f.defaultValue;
        if (member?.jiraUserId && v.fields.some((f) => f.id === "assignee"))
          initial.assignee = v.cloud
            ? { accountId: member.jiraUserId }
            : { name: member.jiraLogin };
        setValues(initial);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [project, type, member?.jiraUserId, member?.jiraLogin, retry]);
  async function addFiles(list: FileList | null) {
    if (!list) return;
    setFileError("");
    setReading(true);
    try {
      const selected = Array.from(list);
      if (
        files.length + selected.length > 10 ||
        files.reduce((n, f) => n + f.data.length * 0.75, 0) +
          selected.reduce((n, f) => n + f.size, 0) >
          5 * 1024 * 1024
      )
        throw new Error(
          "До 10 файлов, суммарно до 5 МБ. Более крупные вложения можно добавить в Jira.",
        );
      const uploads = await Promise.all(
        selected.map(
          (file) =>
            new Promise<Upload>((resolve, reject) => {
              const reader = new FileReader();
              reader.onload = () =>
                resolve({
                  name: file.name,
                  data: String(reader.result).split(",")[1],
                });
              reader.onerror = () =>
                reject(new Error("Не удалось прочитать " + file.name));
              reader.readAsDataURL(file);
            }),
        ),
      );
      setFiles((old) => [...old, ...uploads]);
    } catch (e) {
      setFileError((e as Error).message);
    } finally {
      setReading(false);
    }
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (busy || !meta || reading) return;
    setBusy(true);
    setError("");
    const fields = Object.fromEntries(
      Object.entries(values).filter(
        ([k, v]) =>
          !["project", "issuetype", "attachment"].includes(k) &&
          v !== undefined &&
          v !== "" &&
          !(Array.isArray(v) && !v.length),
      ),
    );
    for (const [key, value] of Object.entries(fields)) {
      if (Array.isArray(value)) fields[key] = value.filter((v) => v !== "");
    }
    if (fields.timetracking)
      fields.timetracking = Object.fromEntries(
        Object.entries(object(fields.timetracking)).filter(([, v]) => v !== ""),
      );
    for (const field of meta.fields) {
      if (
        !uncertain &&
        field.required &&
        !["project", "issuetype", "attachment"].includes(field.id) &&
        (fields[field.id] === undefined ||
          fields[field.id] === null ||
          fields[field.id] === "" ||
          (Array.isArray(fields[field.id]) &&
            !(fields[field.id] as unknown[]).length))
      ) {
        setError("Заполните обязательное поле: " + field.name);
        setBusy(false);
        return;
      }
    }
    if (types.find((t) => t.id === type)?.subtask && !fields.parent) {
      setError("Укажите ключ родительской задачи");
      setBusy(false);
      return;
    }
    const input = uncertain
      ? submitted.current
      : {
          requestId: requestId.current,
          project,
          issueType: type,
          fields,
          attachments: files,
        };
    submitted.current = input;
    try {
      const result = await api<Created>("/jira/issues", "POST", input);
      setCreated(result);
      setUncertain(false);
      onCreated?.(result.task);
      await refresh().catch(() => {});
    } catch (e) {
      setError((e as Error).message);
      // Keep exactly the same payload/request ID after an ambiguous response.
      if (!(e instanceof APIError) || e.status >= 500 || e.status === 409)
        setUncertain(true);
    } finally {
      setBusy(false);
    }
  }
  const subtask = types.find((t) => t.id === type)?.subtask;
  let fields =
    meta?.fields.filter(
      (f) => !["project", "issuetype", "attachment"].includes(f.id),
    ) ?? [];
  if (subtask && !fields.some((f) => f.id === "parent"))
    fields = [
      ...fields,
      {
        id: "parent",
        name: "Родительская задача",
        required: true,
        schema: { type: "issuelink" },
      },
    ];
  const order = [
    "summary",
    "parent",
    "description",
    "assignee",
    "priority",
    "duedate",
    "timetracking",
    "labels",
    "components",
    "fixVersions",
  ];
  fields = [...fields].sort(
    (a, b) =>
      (order.includes(a.id) ? order.indexOf(a.id) : 100) -
      (order.includes(b.id) ? order.indexOf(b.id) : 100),
  );
  return (
    <Modal
      title="Создать задачу в Jira"
      wide
      className="jira-create-modal"
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      {created ? (
        <div className="jira-create-success">
          <h3>Создана {created.task.externalId}</h3>
          <p>{created.task.title}</p>
          <a
            className="button"
            href={created.task.url}
            target="_blank"
            rel="noreferrer"
          >
            Открыть в Jira <ExternalLink size={16} />
          </a>
          {created.warnings?.map((w, i) => (
            <p key={i} className="inline-warning">
              {w}
            </p>
          ))}
          <div className="modal-footer">
            <button className="button primary" onClick={onClose}>
              Готово
            </button>
          </div>
        </div>
      ) : (
        <form onSubmit={submit}>
          <p className="muted">
            Поля и обязательность загружаются из выбранного проекта. После
            создания задача появится в Jira и бэклоге; дальнейшая синхронизация
            учитывает ваш фильтр команды.
          </p>
          <fieldset disabled={busy || uncertain} className="jira-fields">
            <div className="jira-context">
              <label>
                Проект *
                <select
                  aria-label="Проект *"
                  required
                  value={project}
                  onChange={(e) => setProject(e.target.value)}
                >
                  <option value="">Выберите проект</option>
                  {projects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.key} · {p.name}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                Тип задачи *
                <select
                  aria-label="Тип задачи *"
                  required
                  value={type}
                  disabled={!project || !types.length}
                  onChange={(e) => setType(e.target.value)}
                >
                  <option value="">Выберите тип</option>
                  {types.map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.name}
                      {t.subtask ? " (подзадача)" : ""}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            {fields.map((f) => (
              <div
                className={
                  "jira-field " +
                  (["summary", "description", "environment"].includes(f.id)
                    ? "full"
                    : "")
                }
                key={f.id}
              >
                <label htmlFor={"jira-" + f.id}>
                  {f.name}
                  {f.required ? " *" : ""}
                </label>
                <JiraField
                  field={f}
                  value={values[f.id]}
                  change={(v) => setValues((old) => ({ ...old, [f.id]: v }))}
                  cloud={!!meta?.cloud}
                  project={project}
                  member={member}
                  disabled={busy || uncertain}
                />
              </div>
            ))}
            {meta && (
              <div className="jira-field full">
                <label>
                  Вложения · до 5 МБ суммарно
                  <input
                    aria-label="Вложения Jira"
                    type="file"
                    multiple
                    disabled={reading}
                    onChange={(e) => {
                      void addFiles(e.target.files);
                      e.target.value = "";
                    }}
                  />
                </label>
                {files.map((f, i) => (
                  <div className="jira-upload" key={i}>
                    <span>{f.name}</span>
                    <button
                      type="button"
                      className="icon-button"
                      aria-label={"Убрать " + f.name}
                      onClick={() =>
                        setFiles((old) => old.filter((_, n) => n !== i))
                      }
                    >
                      <X size={16} />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </fieldset>
          {loading && (
            <p role="status" className="muted">
              Загрузка настроек Jira…
            </p>
          )}
          {fileError && (
            <p role="alert" className="inline-warning">
              {fileError}
            </p>
          )}
          {error && (
            <div role="alert" className="inline-warning">
              {error}
              {!meta && (
                <button
                  type="button"
                  className="button small"
                  onClick={() => setRetry((n) => n + 1)}
                >
                  Повторить
                </button>
              )}
            </div>
          )}
          {uncertain && (
            <p className="inline-warning">
              Результат запроса неизвестен. Кнопка ниже проверит сохранённую
              операцию без повторного создания. Перед новой попыткой через
              другую форму проверьте Jira.
            </p>
          )}
          <div className="jira-create-footnote">
            Статусы после создания определяются процессом Jira. Специальные поля
            плагинов можно заполнить в формате JSON или через полную форму Jira.{" "}
            {meta && (
              <a href={meta.url} target="_blank" rel="noreferrer">
                Открыть форму Jira ↗
              </a>
            )}
          </div>
          <div className="modal-footer">
            <button
              type="button"
              className="button"
              disabled={busy}
              onClick={onClose}
            >
              Закрыть
            </button>
            <button
              className="button primary"
              disabled={busy || loading || !meta || reading}
            >
              <Plus size={16} />
              {busy
                ? "Создание…"
                : uncertain
                  ? "Проверить результат"
                  : "Создать в Jira"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}

function JiraField({
  field: f,
  value,
  change,
  cloud,
  project,
  member,
  disabled,
}: {
  field: Field;
  value: unknown;
  change: (v: unknown) => void;
  cloud: boolean;
  project: string;
  member?: Member;
  disabled: boolean;
}) {
  const id = "jira-" + f.id,
    type = f.schema.type,
    items = f.schema.items;
  const opts = f.allowedValues ?? [];
  if (type === "user" || (type === "array" && items === "user"))
    return (
      <JiraUserField
        id={id}
        value={value}
        change={change}
        cloud={cloud}
        project={project}
        assignable={f.id === "assignee"}
        multiple={type === "array"}
        required={f.required}
        member={member}
      />
    );
  if (type === "timetracking" || f.id === "timetracking")
    return (
      <div className="jira-context">
        <label>
          Первоначальная оценка
          <input
            id={id}
            placeholder="Например, 2d 4h"
            aria-label="Первоначальная оценка"
            value={formatHours(object(value).originalEstimate)}
            onChange={(e) =>
              change({ ...object(value), originalEstimate: e.target.value })
            }
          />
        </label>
        <label>
          Осталось
          <input
            placeholder="Например, 4h"
            aria-label="Осталось"
            value={formatHours(object(value).remainingEstimate)}
            onChange={(e) =>
              change({ ...object(value), remainingEstimate: e.target.value })
            }
          />
        </label>
      </div>
    );
  if (opts.length) {
    const multiple = type === "array";
    const selected = multiple
      ? Array.isArray(value)
        ? value.map((v) => optionKey(object(v)))
        : []
      : optionKey(object(value));
    const parent = opts.find((o) => optionKey(o) === selected);
    const children = Array.isArray(parent?.children)
      ? (parent.children as Value[])
      : [];
    return (
      <>
        <select
          id={id}
          required={f.required}
          multiple={multiple}
          size={multiple ? Math.min(5, opts.length) : undefined}
          value={selected}
          onChange={(e) =>
            change(
              multiple
                ? Array.from(e.target.selectedOptions).map((o) =>
                    optionValue(opts.find((v) => optionKey(v) === o.value)!),
                  )
                : e.target.value
                  ? optionValue(
                      opts.find((v) => optionKey(v) === e.target.value)!,
                    )
                  : undefined,
            )
          }
        >
          {!multiple && <option value="">Не выбрано</option>}
          {opts.map((o, i) => (
            <option key={optionKey(o) || i} value={optionKey(o)}>
              {labelOf(o)}
            </option>
          ))}
        </select>
        {multiple && (
          <small className="muted">
            Несколько значений: Ctrl / ⌘ + нажатие
          </small>
        )}
        {children.length > 0 && (
          <select
            aria-label={f.name + " — уточнение"}
            value={optionKey(object(object(value).child))}
            onChange={(e) =>
              change({
                ...object(value),
                child: e.target.value
                  ? optionValue(
                      children.find((v) => optionKey(v) === e.target.value)!,
                    )
                  : undefined,
              })
            }
          >
            <option value="">Без уточнения</option>
            {children.map((o) => (
              <option key={optionKey(o)} value={optionKey(o)}>
                {labelOf(o)}
              </option>
            ))}
          </select>
        )}
      </>
    );
  }
  if (f.id === "parent" || type === "issuelink")
    return (
      <input
        id={id}
        required={f.required}
        placeholder="PROJECT-123"
        value={String(object(value).key ?? "")}
        onChange={(e) =>
          change(
            e.target.value
              ? { key: e.target.value.trim().toUpperCase() }
              : undefined,
          )
        }
      />
    );
  if (
    f.id === "description" ||
    f.id === "environment" ||
    f.schema.custom?.endsWith(":textarea")
  )
    return (
      <JiraDescription
        id={id}
        name={f.name}
        value={value}
        cloud={cloud}
        onChange={change}
        disabled={disabled}
      />
    );
  if (type === "array" && items === "string")
    return (
      <input
        id={id}
        required={f.required}
        placeholder="Значения через запятую"
        value={Array.isArray(value) ? value.join(", ") : ""}
        onChange={(e) => change(e.target.value.split(",").map((v) => v.trim()))}
      />
    );
  if (["string", "number", "date", "datetime"].includes(type || ""))
    return (
      <input
        id={id}
        required={f.required}
        type={type === "number" ? "number" : type === "date" ? "date" : "text"}
        step="any"
        placeholder={
          type === "datetime" ? "2026-11-09T10:00:00.000+0300" : undefined
        }
        maxLength={f.id === "summary" ? 255 : undefined}
        value={String(value ?? "")}
        onChange={(e) =>
          change(
            type === "number"
              ? e.target.value === ""
                ? undefined
                : Number(e.target.value)
              : e.target.value,
          )
        }
      />
    );
  return (
    <JsonField id={id} required={f.required} value={value} change={change} />
  );
}
function JsonField({
  id,
  required,
  value,
  change,
}: {
  id: string;
  required: boolean;
  value: unknown;
  change: (v: unknown) => void;
}) {
  const [text, setText] = useState(
    value === undefined ? "" : JSON.stringify(value, null, 2),
  );
  return (
    <>
      <textarea
        id={id}
        required={required}
        rows={3}
        value={text}
        placeholder='JSON значения поля, например {"id":"123"}'
        onChange={(e) => {
          const raw = e.target.value;
          setText(raw);
          try {
            change(raw.trim() ? JSON.parse(raw) : undefined);
            e.target.setCustomValidity("");
          } catch {
            e.target.setCustomValidity(
              "Введите корректный JSON или используйте форму Jira",
            );
          }
        }}
      />
      <small className="muted">
        Специальное поле Jira. Требуется значение в формате его REST API.
      </small>
    </>
  );
}
function JiraUserField({
  id,
  value,
  change,
  cloud,
  project,
  assignable,
  multiple,
  required,
  member,
}: {
  id: string;
  value: unknown;
  change: (v: unknown) => void;
  cloud: boolean;
  project: string;
  assignable: boolean;
  multiple: boolean;
  required: boolean;
  member?: Member;
}) {
  const [search, setSearch] = useState(""),
    [users, setUsers] = useState<JiraUser[]>([]),
    [error, setError] = useState("");
  const term = useDebounced(search);
  useEffect(() => {
    const c = new AbortController();
    setError("");
    if (!assignable && term.length < 2) return;
    void api<JiraUser[]>(
      assignable
        ? `/jira/create/assignees?project=${endpoint(project)}&search=${endpoint(term)}`
        : `/jira/users?search=${endpoint(term)}`,
      "GET",
      undefined,
      c.signal,
    )
      .then(setUsers)
      .catch((e) => {
        if (!c.signal.aborted) setError(e.message);
      });
    return () => c.abort();
  }, [assignable, project, term]);
  const key = cloud ? "accountId" : "name";
  const selected = multiple
    ? Array.isArray(value)
      ? value.map((v) => String(object(v)[key] ?? ""))
      : []
    : String(object(value)[key] ?? "");
  const options = [...users];
  for (const selectedID of Array.isArray(selected)
    ? selected
    : selected
      ? [selected]
      : [])
    if (!options.some((u) => (cloud ? u.id : u.login) === selectedID))
      options.unshift({
        id: cloud ? selectedID : "",
        login: selectedID,
        name:
          member && [member.jiraUserId, member.jiraLogin].includes(selectedID)
            ? member.name
            : selectedID,
        email: "",
      });
  return (
    <div className="jira-user-picker">
      <input
        aria-label="Поиск пользователя Jira"
        placeholder="Поиск по имени, логину или email"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
      <select
        id={id}
        required={required}
        multiple={multiple}
        value={selected}
        onChange={(e) =>
          change(
            multiple
              ? Array.from(e.target.selectedOptions).map((o) => ({
                  [key]: o.value,
                }))
              : e.target.value
                ? { [key]: e.target.value }
                : undefined,
          )
        }
      >
        {!multiple && <option value="">По умолчанию в Jira</option>}
        {options.map((u) => (
          <option key={u.id || u.login} value={cloud ? u.id : u.login}>
            {u.name} · {u.login}
          </option>
        ))}
      </select>
      {error && (
        <small role="alert" className="overdue">
          {error}
        </small>
      )}
      <small className="muted">
        До 50 совпадений. Уточните поиск, если нужного пользователя нет.
      </small>
    </div>
  );
}

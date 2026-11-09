import { useState, type FormEvent } from "react";
import { Trash2, Save, ExternalLink } from "lucide-react";
import { useData } from "../../app/store";
import { api, dateKey, endpoint } from "../../shared/api";
import { Modal } from "../../shared/ui";
import { statuses, priorities, type Task } from "../../shared/types";
export function TaskEditor({
  task,
  personal = false,
  plannedDate,
  onClose,
}: {
  task?: Task;
  personal?: boolean;
  plannedDate?: string;
  onClose: () => void;
}) {
  const { state, act } = useData();
  const [form, setForm] = useState<Task>(
    task || {
      id: "",
      title: "",
      description: "",
      status: "todo",
      priority: "medium",
      source: "local",
      project: personal ? "Личное" : "",
      assigneeId: personal
        ? state.members.find((m) => m.id === "me")?.id || ""
        : "",
      estimate: 1,
      dueDate: "",
      plannedDate: plannedDate || (personal ? dateKey() : ""),
      personal,
      tags: [],
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      demo: state.settings.demo,
    },
  );
  const [busy, setBusy] = useState(false);
  const [remove, setRemove] = useState(false);
  const update = <K extends keyof Task>(key: K, value: Task[K]) =>
    setForm((f) => ({ ...f, [key]: value }));
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await act(
        () =>
          api(
            "/tasks" + (task ? "/" + endpoint(task.id) : ""),
            task ? "PUT" : "POST",
            form,
          ),
        "Задача сохранена",
      );
      onClose();
    } catch {
    } finally {
      setBusy(false);
    }
  }
  async function destroy() {
    setBusy(true);
    try {
      await act(
        () => api("/tasks/" + endpoint(form.id), "DELETE"),
        "Задача удалена",
      );
      onClose();
    } catch {
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      title={
        task
          ? "Карточка задачи"
          : personal
            ? "Новая личная задача"
            : "Новая задача"
      }
      onClose={onClose}
    >
      <form onSubmit={submit} className="form">
        <label>
          Название
          <input
            autoFocus
            required
            maxLength={300}
            value={form.title}
            onChange={(e) => update("title", e.target.value)}
            placeholder="Что нужно сделать?"
          />
        </label>
        <label>
          Описание и критерии готовности
          <textarea
            rows={5}
            value={form.description}
            onChange={(e) => update("description", e.target.value)}
            placeholder="Контекст, ожидаемый результат, полезные ссылки…"
          />
        </label>
        <div className="form-grid">
          <label>
            Статус
            <select
              aria-label="Статус"
              value={form.status}
              onChange={(e) =>
                update("status", e.target.value as Task["status"])
              }
            >
              {Object.entries(statuses).map(([key, name]) => (
                <option key={key} value={key}>
                  {name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Приоритет
            <select
              aria-label="Приоритет"
              value={form.priority}
              onChange={(e) =>
                update("priority", e.target.value as Task["priority"])
              }
            >
              {Object.entries(priorities).map(([key, name]) => (
                <option key={key} value={key}>
                  {name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Исполнитель
            <select
              aria-label="Исполнитель"
              value={form.assigneeId}
              onChange={(e) => update("assigneeId", e.target.value)}
            >
              <option value="">Не назначен</option>
              {state.members
                .filter((m) => !m.inactive || m.id === form.assigneeId)
                .map((m) => (
                  <option key={m.id} value={m.id} disabled={m.inactive}>
                    {m.name}
                    {m.inactive ? " · вне команды" : ""}
                  </option>
                ))}
            </select>
          </label>
          <label>
            Проект
            <input
              maxLength={120}
              value={form.project}
              onChange={(e) => update("project", e.target.value)}
              placeholder="Название проекта"
            />
          </label>
          {form.personal && (
            <label>
              Запланировано на
              <input
                type="date"
                value={form.plannedDate}
                onChange={(e) => update("plannedDate", e.target.value)}
              />
            </label>
          )}
          <label>
            Срок выполнения
            <input
              type="date"
              value={form.dueDate}
              onChange={(e) => update("dueDate", e.target.value)}
            />
          </label>
          <label>
            Оценка, часы
            <input
              type="number"
              min={0}
              max={1000}
              step="0.5"
              value={form.estimate}
              onChange={(e) => update("estimate", Number(e.target.value))}
            />
          </label>
          <label>
            Метки через запятую
            <input
              value={form.tags.join(", ")}
              onChange={(e) =>
                update(
                  "tags",
                  e.target.value.split(",").map((s) => s.trim()),
                )
              }
              placeholder="релиз, технический долг"
            />
          </label>
        </div>
        {!form.personal && (
          <p className="muted">
            План по дням собирается во вкладке «Загрузка команды»: нажмите на
            день сотрудника и выберите эту задачу.
          </p>
        )}
        {form.source !== "local" && (
          <div className="info-strip">
            Изменения хранятся в LeadHelper. При синхронизации поля источника
            обновятся; дата вашего плана сохранится.
            {form.url && (
              <a href={form.url} target="_blank" rel="noreferrer">
                Открыть источник <ExternalLink size={14} />
              </a>
            )}
          </div>
        )}
        <div className="modal-footer">
          {task && task.source === "local" && (
            <button
              className="button danger-quiet"
              type="button"
              disabled={busy}
              onClick={() => (remove ? void destroy() : setRemove(true))}
            >
              <Trash2 size={16} />
              {remove ? "Подтвердить удаление" : "Удалить"}
            </button>
          )}
          <span className="spacer" />
          <button type="button" className="button" onClick={onClose}>
            Отмена
          </button>
          <button className="button primary" disabled={busy}>
            <Save size={16} />
            {busy ? "Сохранение…" : "Сохранить"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

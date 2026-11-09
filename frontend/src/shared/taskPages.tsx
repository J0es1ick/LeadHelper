import { useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useData } from "../app/store";
import { api } from "./api";
import { poll, unchanged } from "./live";
import type { TaskPage } from "./types";
import "./taskPages.css";
export function useDebounced(value: string) {
  const [result, setResult] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setResult(value), 250);
    return () => clearTimeout(timer);
  }, [value]);
  return result;
}
export type TaskFilters = Record<string, string | number>;
export function useTaskPage(filters: TaskFilters, enabled = true) {
  const { state } = useData();
  const key = new URLSearchParams(
    Object.entries(filters).map(([k, v]) => [k, String(v)]),
  ).toString();
  const [page, setPage] = useState(1),
    [retry, setRetry] = useState(0);
  const [data, setData] = useState<TaskPage | null>(null),
    [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const loaded = useRef("");
  // Remount callers on filter changes where page reset is required; this reset also
  // supports date/search changes in shared consumers.
  const [lastKey, setLastKey] = useState(key);
  const currentPage = lastKey === key ? page : 1;
  useEffect(() => {
    setLastKey(key);
    setPage(1);
  }, [key]);
  useEffect(() => {
    if (!enabled) {
      setData(null);
      setLoading(false);
      return;
    }
    const c = new AbortController();
    const requestKey = `${key}:${currentPage}:${state.settings.demo}`;
    setLoading(loaded.current !== requestKey);
    setError("");
    if (loaded.current !== requestKey) setData(null);
    poll(
      () =>
        api<TaskPage>(
          `/tasks?${key}&page=${currentPage}`,
          "GET",
          undefined,
          c.signal,
        )
          .then((p) => {
            if (c.signal.aborted) return;
            setData((old) => unchanged(old, p));
            setPage(p.page);
            loaded.current = requestKey;
            setError("");
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
  }, [key, currentPage, state.revision, state.settings.demo, enabled, retry]);
  return { data, loading, error, setPage, retry: () => setRetry((n) => n + 1) };
}
export function TaskPagination({
  data,
  onPage,
  disabled = false,
}: {
  data: Pick<TaskPage, "page" | "pages" | "total" | "limit"> | null;
  onPage: (p: number) => void;
  disabled?: boolean;
}) {
  if (!data) return null;
  return (
    <nav className="task-pagination" aria-label="Страницы задач">
      <span>
        {data.total
          ? `${(data.page - 1) * data.limit + 1}–${Math.min(data.page * data.limit, data.total)} из ${data.total}`
          : "0 задач"}
      </span>
      <div>
        <button
          className="icon-button"
          aria-label="Предыдущая страница задач"
          disabled={disabled || data.page <= 1}
          onClick={() => onPage(data.page - 1)}
        >
          <ChevronLeft size={16} />
        </button>
        <span>
          {data.page} / {data.pages}
        </span>
        <button
          className="icon-button"
          aria-label="Следующая страница задач"
          disabled={disabled || data.page >= data.pages}
          onClick={() => onPage(data.page + 1)}
        >
          <ChevronRight size={16} />
        </button>
      </div>
    </nav>
  );
}
export function TaskPageFeedback({
  loading,
  error,
  retry,
}: {
  loading: boolean;
  error: string;
  retry: () => void;
}) {
  return loading ? (
    <p className="muted pad" role="status">
      Загрузка задач…
    </p>
  ) : error ? (
    <div className="inline-warning" role="alert">
      {error}
      <button className="button small" onClick={retry}>
        Повторить
      </button>
    </div>
  ) : null;
}

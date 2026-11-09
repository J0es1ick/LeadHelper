import { useEffect, useRef, useState } from "react";
import { api } from "../../shared/api";
import { poll } from "../../shared/live";
import { Modal } from "../../shared/ui";

type SyncStatus = {
  configured: boolean;
  title: string;
  spreadsheetId: string;
  updatedAt: string;
  error?: string;
  pending: number;
  conflicts: number;
};
export function ProjectSheetSync({ changed }: { changed: () => void }) {
  const changedRef = useRef(changed);
  changedRef.current = changed;
  const lastUpdate = useRef("");
  const [status, setStatus] = useState<SyncStatus | null>(null),
    [open, setOpen] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const load = (signal?: AbortSignal) =>
    api<SyncStatus>("/projects/sync", "GET", undefined, signal)
      .then((value) => {
        if (signal?.aborted) return;
        if (lastUpdate.current && value.updatedAt !== lastUpdate.current)
          changedRef.current();
        lastUpdate.current = value.updatedAt;
        setStatus(value);
        setError("");
      })
      .catch((e) => {
        if (!signal?.aborted) setError(e.message);
        throw e;
      });
  useEffect(() => {
    const c = new AbortController();
    poll(() => load(c.signal), { signal: c.signal, interval: 10000 });
    return () => c.abort();
  }, []);
  async function sync() {
    setBusy(true);
    setError("");
    try {
      await api("/projects/sync", "POST");
      changed();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      await load().catch(() => {});
      setBusy(false);
    }
  }
  return (
    <section className="project-sheet-sync">
      <div>
        <strong>{status?.title || "Google Таблица проектов"}</strong>
        <p>
          {status?.configured
            ? "Обмен в обе стороны · проверка каждую минуту"
            : status?.spreadsheetId
              ? "Таблица импортирована. Для фонового обмена подключите синхронизацию."
              : "Подключите таблицу для обмена статусами и проектами."}
        </p>
        {!!status?.pending && <span>Ожидают отправки: {status.pending}. </span>}
        {!!status?.conflicts && (
          <span>Требуют проверки: {status.conflicts}. </span>
        )}
        {status?.updatedAt && !status.updatedAt.startsWith("0001") && (
          <small>
            Данные на {new Date(status.updatedAt).toLocaleString("ru-RU")}
          </small>
        )}
      </div>
      <div className="project-actions">
        <button className="button" onClick={() => setOpen(true)}>
          {status?.configured ? "Подключение" : "Подключить синхронизацию"}
        </button>
        {status?.configured && (
          <button
            className="button"
            disabled={busy}
            onClick={() => void sync()}
          >
            {busy ? "Синхронизация…" : "Обновить из таблицы"}
          </button>
        )}
      </div>
      {(error || status?.error) && (
        <p className="inline-warning" role="alert">
          {error || status?.error}
        </p>
      )}
      {open && (
        <SheetConnection
          close={() => setOpen(false)}
          saved={() => {
            setOpen(false);
            void sync();
          }}
        />
      )}
    </section>
  );
}
function SheetConnection({
  close,
  saved,
}: {
  close: () => void;
  saved: () => void;
}) {
  const [url, setURL] = useState(""),
    [token, setToken] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  useEffect(() => {
    const a = new AbortController();
    void api<{ url: string }>(
      "/connections/google-sheets",
      "GET",
      undefined,
      a.signal,
    )
      .then((v) => setURL(v.url || ""))
      .catch(() => {});
    return () => a.abort();
  }, []);
  return (
    <Modal title="Синхронизация Google Таблицы" onClose={close} wide>
      <div className="project-sheet-setup">
        <p>
          Один раз настройте скрипт от Google-аккаунта с правом редактирования
          таблицы. После этого обмен работает, пока запущен LeadHelper, даже с
          закрытой вкладкой.
        </p>
        <ol>
          <li>
            В исходной таблице откройте <b>Расширения → Apps Script</b>.
          </li>
          <li>
            <a href="/project-sheets.gs" download>
              Скачайте скрипт LeadHelper
            </a>
            , скопируйте его содержимое в редактор и сохраните.
          </li>
          <li>
            Слева нажмите <b>Сервисы + → Google Sheets API → Добавить</b>.
          </li>
          <li>
            Выберите функцию <b>setupLeadHelper</b>, нажмите «Выполнить» и
            разрешите доступ. В таблице появится окно с секретом — вставьте его
            ниже.
          </li>
          <li>
            <b>Начать развёртывание → Новое развёртывание → Веб-приложение</b>.
            Запуск от вашего имени, доступ «Все». Скопируйте URL,
            заканчивающийся на <b>/exec</b>.
          </li>
        </ol>
        <p className="muted">
          Сама таблица остаётся закрытой. Доступ к скрипту защищён секретом.
          Названия и новые статусы записываются в исходные листы; дополнительные
          поля — в служебный лист «__LeadHelper», новые проекты — в «Проекты
          LeadHelper». История статусов сохраняется.
        </p>
        <form
          className="project-form"
          onSubmit={(e) => {
            e.preventDefault();
            setBusy(true);
            setError("");
            void api("/connections/google-sheets", "PUT", { url, token })
              .then(saved)
              .catch((e) => setError(e.message))
              .finally(() => setBusy(false));
          }}
        >
          <label>
            URL веб-приложения
            <input
              type="url"
              required
              value={url}
              onChange={(e) => setURL(e.target.value)}
              placeholder="https://script.google.com/macros/s/…/exec"
            />
          </label>
          <label>
            Секрет подключения
            <input
              type="password"
              autoComplete="new-password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="Оставьте пустым, чтобы сохранить текущий"
            />
          </label>
          {error && (
            <p className="inline-warning" role="alert">
              {error}
            </p>
          )}
          <button className="button primary" disabled={busy}>
            {busy ? "Проверка доступа…" : "Проверить и подключить"}
          </button>
        </form>
      </div>
    </Modal>
  );
}

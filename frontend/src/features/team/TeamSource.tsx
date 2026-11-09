import { useState } from "react";
import { Network, RefreshCw, CheckCircle2 } from "lucide-react";
import { useData } from "../../app/store";
import { api } from "../../shared/api";
import type { TeamSyncStatus } from "../../shared/types";

export function TeamSource() {
  const { state, act, refresh } = useData();
  const [busy, setBusy] = useState(false);
  const status: TeamSyncStatus = state.teamSync || {
    enabled: false,
    memberCount: 0,
    departmentCount: 0,
  };
  if (state.settings.demo) return null;
  async function run(mode: "connect" | "refresh" | "pause") {
    setBusy(true);
    try {
      await act(
        () =>
          mode === "refresh"
            ? api("/team/bitrix/sync", "POST")
            : api("/team/bitrix", "PUT", { enabled: mode === "connect" }),
        mode === "pause"
          ? "Автообновление команды приостановлено"
          : "Состав команды обновлён",
      );
    } catch {
      await refresh().catch(() => {});
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="team-source" aria-label="Состав команды из Битрикса">
      <div className="team-source-heading">
        <span className="team-source-icon">
          <Network size={21} />
        </span>
        <div>
          <h2>Команда из Битрикса</h2>
          <p>
            {status.lastSuccessAt
              ? "Ваши прямые и косвенные подчинённые"
              : "Отделы, которыми вы руководите, и все вложенные подразделения"}
          </p>
        </div>
        <button
          className={"button " + (!status.lastSuccessAt ? "primary" : "")}
          disabled={busy || !state.settings.modules.bitrix}
          onClick={() => void run(status.lastSuccessAt ? "refresh" : "connect")}
        >
          <RefreshCw size={16} className={busy ? "team-sync-spin" : ""} />
          {busy
            ? "Обновляем…"
            : status.lastSuccessAt
              ? "Обновить состав"
              : "Подключить мою команду"}
        </button>
      </div>
      {status.lastSuccessAt && (
        <>
          <div className="team-source-roots">
            {status.roots?.map((d) => (
              <span key={d.id}>
                <Network size={14} />
                {d.name}
              </span>
            ))}
          </div>
          <div className="team-source-meta">
            <span>
              <CheckCircle2 size={15} />
              {status.memberCount} сотрудников · подразделений:{" "}
              {status.departmentCount}
            </span>
            <span>
              Обновлено{" "}
              {new Date(status.lastSuccessAt).toLocaleString("ru-RU", {
                timeZone: "Europe/Moscow",
                day: "numeric",
                month: "short",
                hour: "2-digit",
                minute: "2-digit",
              })}{" "}
              МСК
            </span>
          </div>
        </>
      )}
      {(status.lastSuccessAt || status.enabled) && (
        <label className="team-auto-toggle">
          <input
            type="checkbox"
            checked={status.enabled}
            disabled={busy}
            onChange={(e) => void run(e.target.checked ? "connect" : "pause")}
          />
          <span>Автоматически обновлять каждую минуту</span>
        </label>
      )}
      <p className="team-source-hint">
        {status.lastSuccessAt
          ? "Новые сотрудники добавляются с доступностью 32 ч/нед. Настройте часы каждого участника; обновление состава сохранит ваши значения."
          : "Имена, должности и подразделения загружаются автоматически. Доступные часы задаются в LeadHelper."}
      </p>
      {!state.settings.modules.bitrix && (
        <p className="inline-warning">
          Включите модуль Битрикс24 в настройках, чтобы обновлять состав.
        </p>
      )}
      {status.error && (
        <div className="inline-warning" role="alert">
          {status.error}
          {status.lastSuccessAt && (
            <span> Показан последний успешно загруженный состав.</span>
          )}
        </div>
      )}
    </section>
  );
}

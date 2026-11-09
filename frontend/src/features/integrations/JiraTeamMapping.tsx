import { useEffect, useState } from "react";
import { Link2, RefreshCw, Search } from "lucide-react";
import { useData } from "../../app/store";
import { api, endpoint } from "../../shared/api";
import { Modal } from "../../shared/ui";
import type { Member } from "../../shared/types";
import "./jira-team.css";

type JiraUser = { id: string; login: string; name: string; email?: string };
type TeamStatus = {
  scope: string;
  site: string;
  members: Member[];
  linked: number;
};
export function JiraTeamMapping() {
  const { state, act, refresh } = useData();
  const [status, setStatus] = useState<TeamStatus | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [selected, setSelected] = useState<Member | null>(null);
  const load = () =>
    api<TeamStatus>("/jira/team").then((v) => {
      setStatus(v);
      setError("");
    });
  useEffect(() => {
    let stopped = false;
    api<TeamStatus>("/jira/team")
      .then((v) => {
        if (!stopped) setStatus(v);
      })
      .catch((e) => {
        if (!stopped) setError(e.message);
      });
    return () => {
      stopped = true;
    };
  }, [state.members, state.syncLogs]);
  async function match() {
    setBusy(true);
    setError("");
    try {
      await act(
        () => api("/jira/team/match", "POST"),
        "Сотрудники сопоставлены",
      );
      await load();
    } catch (e) {
      setError((e as Error).message);
      await refresh().catch(() => {});
    } finally {
      setBusy(false);
    }
  }
  return (
    <section
      className="panel jira-team-panel"
      aria-label="Команда и исполнители Jira"
    >
      <div className="jira-team-heading">
        <div>
          <h2>Команда и исполнители Jira</h2>
          <p>
            {status?.scope === "jql"
              ? "Включена собственная выборка JQL. Чтобы использовать команду, смените режим в настройках Jira."
              : "Загружаются только задачи действующих сотрудников из Битрикса, из любых доступных проектов."}
          </p>
        </div>
        <span className="count-badge">
          {status?.linked || 0} / {status?.members.length || 0}
        </span>
      </div>
      {status?.scope !== "jql" && (
        <>
          <p className="jira-team-help">
            {status &&
            status.members.length > 0 &&
            status.linked === status.members.length
              ? "Все сотрудники сопоставлены. Состав команды учитывается при каждой синхронизации; задачи появятся в бэклоге без повторных карточек сотрудников."
              : "Точное совпадение email связывается автоматически. Если адреса различаются или скрыты, выберите пользователя вручную. Пока сопоставление не закончено, загрузка задач приостановлена."}
          </p>
          <button
            className="button"
            disabled={busy || !status?.members.length}
            onClick={() => void match()}
          >
            <RefreshCw size={15} />
            {busy ? "Сопоставляем…" : "Сопоставить по email"}
          </button>
          {!status?.members.length && (
            <p className="jira-team-help">
              Сначала подключите состав в разделе «Загрузка команды».
            </p>
          )}
          <div className="jira-team-list">
            {status?.members.map((m) => {
              const linked = m.jiraSite === status.site && !!m.jiraUserId;
              return (
                <div className="jira-team-row" key={m.id}>
                  <div>
                    <strong>{m.name}</strong>
                    <small>{m.email || "Email не указан в Битриксе"}</small>
                  </div>
                  <div className={linked ? "jira-team-linked" : "muted"}>
                    {linked ? (
                      <>
                        <Link2 size={14} />
                        <span>
                          {m.jiraName || m.jiraLogin}
                          <small>{m.jiraLogin}</small>
                        </span>
                      </>
                    ) : (
                      "Не сопоставлен"
                    )}
                  </div>
                  <button
                    className="button small"
                    onClick={() => setSelected(m)}
                    aria-label={"Выбрать пользователя Jira для " + m.name}
                  >
                    {linked ? "Изменить" : "Выбрать"}
                  </button>
                </div>
              );
            })}
          </div>
        </>
      )}
      {error && (
        <div className="inline-warning" role="alert">
          {error}
        </div>
      )}
      {selected && (
        <JiraUserPicker
          member={selected}
          close={() => setSelected(null)}
          saved={async () => {
            setSelected(null);
            await refresh();
            await load();
          }}
        />
      )}
    </section>
  );
}
function JiraUserPicker({
  member,
  close,
  saved,
}: {
  member: Member;
  close: () => void;
  saved: () => Promise<void>;
}) {
  const [search, setSearch] = useState(member.email || member.name);
  const [users, setUsers] = useState<JiraUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    let stopped = false;
    setLoading(true);
    setError("");
    setUsers([]);
    const timer = setTimeout(() => {
      if (search.trim().length < 2) {
        setLoading(false);
        return;
      }
      api<JiraUser[]>(
        "/jira/users?" + new URLSearchParams({ search: search.trim() }),
      )
        .then((v) => {
          if (!stopped) setUsers(v);
        })
        .catch((e) => {
          if (!stopped) setError(e.message);
        })
        .finally(() => {
          if (!stopped) setLoading(false);
        });
    }, 300);
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [search]);
  async function link(u: JiraUser) {
    setBusy(true);
    setError("");
    try {
      await api("/jira/team/" + endpoint(member.id), "PUT", { userId: u.id });
      await saved();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal title="Пользователь Jira" onClose={busy ? () => {} : close}>
      <div className="form jira-user-picker">
        <p>
          Сотрудник в Битриксе: <strong>{member.name}</strong>
        </p>
        <label>
          Имя, логин или email в Jira
          <input
            value={search}
            disabled={busy}
            onChange={(e) => setSearch(e.target.value)}
          />
        </label>
        {error && (
          <div className="inline-warning" role="alert">
            {error}
          </div>
        )}
        {loading ? (
          <p className="muted">Ищем пользователей…</p>
        ) : (
          !users.length && (
            <p className="muted">
              Нет совпадений. Попробуйте рабочий email или логин.
            </p>
          )
        )}
        <div className="jira-user-results">
          {users.map((u) => (
            <button
              key={u.id}
              className="jira-user-result"
              disabled={busy}
              onClick={() => void link(u)}
            >
              <Search size={16} />
              <span>
                <strong>{u.name}</strong>
                <small>
                  {u.login}
                  {u.email ? " · " + u.email : ""}
                </small>
              </span>
              <span>Связать</span>
            </button>
          ))}
        </div>
      </div>
    </Modal>
  );
}

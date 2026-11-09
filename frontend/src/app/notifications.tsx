import { useEffect, useRef, useState } from "react";
import { Bell, CheckCheck, Mail, MessageCircle } from "lucide-react";
import { useData } from "./store";
import { api } from "../shared/api";
import { Empty, Modal } from "../shared/ui";
import type { InboxNotification } from "../shared/types";
import { plainRemoteText } from "../features/integrations/bitrix-model";

function notificationText(n: InboxNotification, field: "title" | "text") {
  return n.provider === "bitrix" ? plainRemoteText(n[field]) : n[field];
}

const preference = "leadhelper.notifications.desktop";
function enabled() {
  try {
    return localStorage.getItem(preference) === "on";
  } catch {
    return false;
  }
}
export function useDesktopNotifications(
  items: InboxNotification[] | undefined,
  demo: boolean,
) {
  const baseline = useRef(false);
  const seen = useRef(new Set<string>());
  const popup = useRef<{ notice: Notification; ids: string[] } | null>(null);
  useEffect(() => () => popup.current?.notice.close(), []);
  useEffect(() => {
    if (
      popup.current &&
      (demo ||
        !popup.current.ids.some((id) =>
          items?.some((n) => n.id === id && !n.read),
        ))
    ) {
      popup.current.notice.close();
      popup.current = null;
    }
    if (!items || demo) {
      baseline.current = false;
      return;
    }
    const fresh = items.filter((n) => !n.read && !seen.current.has(n.id));
    const initial = !baseline.current;
    baseline.current = true;
    seen.current = new Set(items.map((n) => n.id));
    if (
      initial ||
      !enabled() ||
      !("Notification" in window) ||
      Notification.permission !== "granted"
    )
      return;
    let shared: string[] = [];
    try {
      shared = JSON.parse(
        localStorage.getItem("leadhelper.notifications.shown") || "[]",
      );
    } catch {}
    const ready = fresh.filter((n) => !shared.includes(n.id));
    if (!ready.length) return;
    try {
      localStorage.setItem(
        "leadhelper.notifications.shown",
        JSON.stringify([...ready.map((n) => n.id), ...shared].slice(0, 400)),
      );
    } catch {}
    try {
      const notice = new Notification(
        ready.length === 1
          ? ready[0].provider === "mail"
            ? "Новое письмо"
            : "Новое сообщение в Битрикс24"
          : `Новых уведомлений: ${ready.length}`,
        {
          body:
            ready.length === 1
              ? `${notificationText(ready[0], "title")}\n${notificationText(ready[0], "text")}`
              : "Откройте уведомления в LeadHelper",
          tag: "leadhelper-incoming",
        },
      );
      popup.current?.notice.close();
      popup.current = { notice, ids: ready.map((n) => n.id) };
      notice.onclick = () => {
        window.focus();
        window.dispatchEvent(new Event("leadhelper:notifications"));
        notice.close();
      };
    } catch {
      /* In-app notifications remain available if the OS blocks popups. */
    }
  }, [items, demo]);
}
export function NotificationSettings() {
  const [on, setOn] = useState(enabled);
  const [message, setMessage] = useState("");
  async function toggle() {
    if (on) {
      try {
        localStorage.setItem(preference, "off");
      } catch {}
      setOn(false);
      return;
    }
    if (!("Notification" in window)) {
      setMessage(
        "Этот браузер не поддерживает системные уведомления. Оповещения доступны по колокольчику.",
      );
      return;
    }
    try {
      const permission = await Notification.requestPermission();
      if (permission !== "granted") {
        setMessage(
          "Разрешите уведомления для localhost в настройках браузера. Внутри сервиса они работают всегда.",
        );
        return;
      }
      localStorage.setItem(preference, "on");
      setOn(true);
      setMessage("");
    } catch {
      setMessage("Не удалось включить уведомления в этом браузере");
    }
  }
  return (
    <div className="notification-settings">
      <div>
        <strong>Уведомления на рабочем столе</strong>
        <p>
          Почта и Битрикс24. Работают, пока сервис запущен и его вкладка
          открыта, в том числе в фоне.
        </p>
        {message && <p role="status">{message}</p>}
      </div>
      <button
        type="button"
        className={"button " + (on ? "" : "primary")}
        onClick={() => void toggle()}
      >
        <Bell size={16} />
        {on ? "Выключить" : "Включить"}
      </button>
    </div>
  );
}
export function Notifications({
  close,
  open,
}: {
  close: () => void;
  open: (n: InboxNotification) => void;
}) {
  const { state, act } = useData();
  const items = (state.notifications || []).filter((n) => !n.read);
  const unread = items.filter((n) => !n.read).length;
  const [filter, setFilter] = useState("all");
  return (
    <Modal
      title={`Уведомления${unread ? ` · ${unread}` : ""}`}
      onClose={close}
      className="notifications-dialog"
    >
      <div className="notifications-body">
        <NotificationSettings />
        <div className="notice-toolbar">
          <div className="segmented">
            {[
              ["all", "Все"],
              ["mail", "Почта"],
              ["bitrix", "Битрикс24"],
            ].map(([id, name]) => (
              <button
                key={id}
                className={filter === id ? "active" : ""}
                onClick={() => setFilter(id)}
              >
                {name}
              </button>
            ))}
          </div>
          <button
            className="text-button"
            disabled={!unread}
            onClick={() =>
              void act(() => api("/notifications/all", "PATCH")).catch(() => {})
            }
          >
            <CheckCheck size={16} />
            Прочитать все
          </button>
        </div>
        <div className="inbox-notifications">
          {items
            .filter((n) => filter === "all" || n.provider === filter)
            .map((n) => (
              <button
                className={"notice-item " + (!n.read ? "unread" : "")}
                key={n.id}
                onClick={() => {
                  open(n);
                  close();
                }}
              >
                {n.provider === "mail" ? (
                  <Mail size={19} />
                ) : (
                  <MessageCircle size={19} />
                )}
                <span>
                  <strong>
                    {notificationText(n, "title") ||
                      (n.provider === "mail" ? "Новое письмо" : "Битрикс24")}
                  </strong>
                  <p>{notificationText(n, "text")}</p>
                  <small>
                    {new Date(n.at).toLocaleString("ru-RU", {
                      timeZone: "Europe/Moscow",
                      day: "numeric",
                      month: "short",
                      hour: "2-digit",
                      minute: "2-digit",
                    })}{" "}
                    МСК
                  </small>
                </span>
                {!n.read && <i />}
              </button>
            ))}
        </div>
        {!items.some((n) => filter === "all" || n.provider === filter) && (
          <Empty
            title="Пока всё спокойно"
            description="Здесь появятся новые письма и сообщения из Битрикса. История первого подключения не вызывает уведомлений."
          />
        )}
      </div>
    </Modal>
  );
}

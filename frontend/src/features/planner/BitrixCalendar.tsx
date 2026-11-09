import { useEffect, useRef, useState } from "react";
import { poll, unchanged } from "../../shared/live";
import {
  ArrowUpRight,
  CalendarDays,
  Clock3,
  MapPin,
  RefreshCw,
} from "lucide-react";
import { api, dayLabel, shiftDay } from "../../shared/api";
import { useData } from "../../app/store";
import type { CalendarPage, CalendarEvent } from "../../shared/types";
import { clock, dayBounds, eventsForDay, occupiedMinutes } from "./calendar";

export function useBitrixCalendar(from: string, to: string) {
  const { state } = useData();
  const [data, setData] = useState<CalendarPage | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [reload, setReload] = useState(0);
  const loaded = useRef("");
  const enabled = state.settings.modules.bitrix;
  const demo = state.settings.demo;
  useEffect(() => {
    if (!enabled) {
      setData(null);
      setError("");
      return;
    }
    if (demo) {
      setError("");
      setLoading(false);
      setData({
        timezone: "Europe/Moscow",
        updatedAt: new Date().toISOString(),
        events: [
          {
            id: "demo-planning",
            title: "Планирование команды",
            start: from + "T10:00:00+03:00",
            end: from + "T11:00:00+03:00",
            busy: true,
            allDay: false,
            tentative: false,
            location: "Переговорная",
            url: "",
          },
          {
            id: "demo-design",
            title: "Обсуждение макетов",
            start: shiftDay(from, 2) + "T14:00:00+03:00",
            end: shiftDay(from, 2) + "T15:30:00+03:00",
            busy: true,
            allDay: false,
            tentative: true,
            location: "Онлайн",
            url: "",
          },
        ],
      });
      return;
    }
    const a = new AbortController();
    const key = `${from}:${to}:${demo}:${enabled}`;
    setLoading(loaded.current !== key);
    async function load() {
      try {
        const value = await api<CalendarPage>(
          "/calendar?from=" + from + "&to=" + to,
          "GET",
          undefined,
          a.signal,
        );
        if (!a.signal.aborted) {
          setData((old) => unchanged(old, value));
          loaded.current = key;
          setError("");
        }
      } catch (e) {
        if (!a.signal.aborted) {
          setError((e as Error).message);
        }
        throw e;
      } finally {
        if (!a.signal.aborted) {
          setLoading(false);
        }
      }
    }
    if (loaded.current !== key) setData(null);
    poll(load, { signal: a.signal, interval: 30000 });
    return () => {
      a.abort();
    };
  }, [from, to, enabled, demo, reload]);
  return {
    data,
    error,
    loading,
    enabled,
    demo,
    refresh: () => setReload((v) => v + 1),
  };
}
export function DayEvent({
  event: e,
  compact = false,
}: {
  event: CalendarEvent;
  compact?: boolean;
}) {
  const label = e.allDay ? "Весь день" : clock(e.start) + "–" + clock(e.end);
  const body = (
    <>
      <span className="calendar-event-time">
        {label}
        {e.tentative
          ? " · Под вопросом"
          : !e.busy
            ? " · Не блокирует время"
            : ""}
      </span>
      <strong>{e.title}</strong>
      {!compact && e.location && (
        <span className="calendar-event-place">
          <MapPin size={13} />
          {e.location}
        </span>
      )}
    </>
  );
  return e.url ? (
    <a
      className={
        "calendar-event " +
        (e.tentative ? "tentative " : "") +
        (e.busy ? "busy" : "free") +
        (compact ? " compact" : "")
      }
      href={e.url}
      target="_blank"
      rel="noreferrer"
      title="Открыть событие в Битрикс24"
    >
      {body}
      {!compact && <ArrowUpRight size={15} />}
    </a>
  ) : (
    <div
      className={
        "calendar-event " +
        (e.tentative ? "tentative " : "") +
        (e.busy ? "busy" : "free") +
        (compact ? " compact" : "")
      }
    >
      {body}
    </div>
  );
}
export function BitrixCalendar({
  day,
  calendar,
}: {
  day: string;
  calendar: ReturnType<typeof useBitrixCalendar>;
}) {
  const { data, error, loading, enabled, demo, refresh } = calendar;
  const events = eventsForDay(data?.events || [], day);
  const minutes = occupiedMinutes(events, day);
  const [a] = dayBounds(day);
  if (!enabled) return null;
  return (
    <section className="panel calendar-panel">
      <div className="section-heading">
        <div>
          <h2>
            <CalendarDays size={18} />
            Занятость · {dayLabel(day)}
          </h2>
          <p>Календарь Битрикс24 · время МСК{demo ? " · Демонстрация" : ""}</p>
        </div>
        <button
          className="icon-button"
          aria-label="Обновить календарь"
          onClick={refresh}
          disabled={loading}
        >
          <RefreshCw size={16} className={loading ? "spin" : ""} />
        </button>
      </div>
      {error ? (
        <div className="calendar-error">
          <p role="alert">{error}</p>
          <a className="text-button" href="#integrations">
            Открыть интеграции
            <ArrowUpRight size={14} />
          </a>
        </div>
      ) : loading && !data ? (
        <p className="muted" role="status">
          Проверяем занятость…
        </p>
      ) : (
        <>
          <div className="calendar-summary">
            <span>
              <Clock3 size={15} />
              {minutes
                ? `Занято: ${Math.floor(minutes / 60)} ч${minutes % 60 ? ` ${minutes % 60} мин` : ""}`
                : "Занятых промежутков нет"}
            </span>
            <small>
              События на выбранный день
              {data?.updatedAt ? " · обновлено " + clock(data.updatedAt) : ""}
            </small>
          </div>
          <div className="day-timeline" aria-label="Занятость с 8 до 20 часов">
            <div className="timeline-labels">
              {[8, 10, 12, 14, 16, 18, 20].map((h) => (
                <span key={h}>{h}:00</span>
              ))}
            </div>
            <div className="timeline-track">
              {events
                .filter((e) => e.busy)
                .map((e) => {
                  const start = e.allDay
                    ? 8
                    : Math.max(8, (Date.parse(e.start) - a) / 3600000);
                  const end = e.allDay
                    ? 20
                    : Math.min(20, (Date.parse(e.end) - a) / 3600000);
                  return end > start ? (
                    <span
                      key={e.id}
                      className={e.tentative ? "tentative" : ""}
                      style={{
                        left: ((start - 8) / 12) * 100 + "%",
                        width: ((end - start) / 12) * 100 + "%",
                      }}
                      title={
                        e.title +
                        " · " +
                        (e.allDay
                          ? "Весь день"
                          : clock(e.start) + "–" + clock(e.end))
                      }
                    />
                  ) : null;
                })}
            </div>
          </div>
          <div className="calendar-events">
            {events.map((e) => (
              <DayEvent key={e.id} event={e} />
            ))}
          </div>
          {!events.length && (
            <p className="calendar-empty">
              В календаре Битрикса на этот день событий нет.
            </p>
          )}
        </>
      )}
    </section>
  );
}

import type { CalendarEvent } from "../../shared/types";
export function dayBounds(day: string) {
  return [
    Date.parse(day + "T00:00:00+03:00"),
    Date.parse(day + "T00:00:00+03:00") + 86400000,
  ] as const;
}
export function eventsForDay(events: CalendarEvent[], day: string) {
  const [a, b] = dayBounds(day);
  return events.filter((e) =>
    e.allDay
      ? e.start.slice(0, 10) <= day && e.end.slice(0, 10) > day
      : Date.parse(e.start) < b && Date.parse(e.end) > a,
  );
}
export function occupiedMinutes(events: CalendarEvent[], day: string) {
  const [a, b] = dayBounds(day);
  const intervals = eventsForDay(events, day)
    .filter((e) => e.busy)
    .map((e) =>
      e.allDay
        ? [a, b]
        : [Math.max(a, Date.parse(e.start)), Math.min(b, Date.parse(e.end))],
    )
    .sort((x, y) => x[0] - y[0]);
  let total = 0,
    end = a;
  for (const [x, y] of intervals) {
    if (y > end) {
      total += y - Math.max(x, end);
      end = y;
    }
  }
  return Math.round(total / 60000);
}
export const clock = (value: string) =>
  new Date(value).toLocaleTimeString("ru-RU", {
    timeZone: "Europe/Moscow",
    hour: "2-digit",
    minute: "2-digit",
  });

export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = "APIError";
  }
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const controller = new AbortController();
  const cancel = () => controller.abort();
  signal?.addEventListener("abort", cancel, { once: true });
  if (signal?.aborted) cancel();
  // A stalled read must not disable every subsequent automatic refresh.
  const timer = method === "GET" ? setTimeout(cancel, 20000) : undefined;
  try {
    const response = await fetch("/api" + path, {
      method,
      signal: controller.signal,
      cache: "no-store",
      headers: { "Content-Type": "application/json", "X-LeadHelper": "1" },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
    if (!response.ok) {
      let message = "Не удалось выполнить действие";
      try {
        message = (await response.json()).error || message;
      } catch {
        /* Non-JSON transport failure. */
      }
      throw new APIError(message, response.status);
    }
    return (await response.json()) as T;
  } catch (e) {
    if (controller.signal.aborted && !signal?.aborted)
      throw new Error(
        "Сервис долго отвечает. Проверка будет повторена автоматически.",
      );
    throw e;
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", cancel);
  }
}
export const endpoint = (id: string) => encodeURIComponent(id);
export const dateKey = (date = new Date()) =>
  new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Moscow",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(date);
export const parseDay = (value: string) => new Date(value + "T12:00:00+03:00");
export function shiftDay(value: string, n: number) {
  const d = parseDay(value);
  d.setDate(d.getDate() + n);
  return dateKey(d);
}
export function weekStart(value = dateKey()) {
  const d = parseDay(value);
  return shiftDay(value, -(d.getDay() + 6) % 7);
}
export const dayLabel = (
  value: string,
  options: Intl.DateTimeFormatOptions = { day: "numeric", month: "short" },
) =>
  value ? parseDay(value).toLocaleDateString("ru-RU", options) : "Без срока";
export const initials = (name: string) =>
  name
    .split(" ")
    .slice(0, 2)
    .map((n) => n[0])
    .join("")
    .toUpperCase();
export function plural(count: number, one: string, few: string, many: string) {
  const n = Math.abs(count) % 100;
  return n > 10 && n < 20
    ? many
    : n % 10 === 1
      ? one
      : n % 10 >= 2 && n % 10 <= 4
        ? few
        : many;
}
export function download(name: string, text: string, type = "text/plain") {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// One request at a time. Hidden views sleep; returning from another tab or an
// offline connection wakes them immediately. Background state feeds notifications.
export function poll(
  load: () => Promise<unknown>,
  {
    signal,
    interval,
    background = false,
    immediate = true,
  }: {
    signal: AbortSignal;
    interval: number;
    background?: boolean;
    immediate?: boolean;
  },
) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let running = false;
  let failures = 0;
  let lastStarted = -Infinity;
  const available = () =>
    navigator.onLine !== false &&
    (background || document.visibilityState !== "hidden");
  function schedule(delay: number) {
    clearTimeout(timer);
    if (!signal.aborted && available())
      timer = setTimeout(() => void run(), delay);
  }
  async function run(initial = false) {
    if (
      signal.aborted ||
      running ||
      navigator.onLine === false ||
      (!initial && !available())
    )
      return;
    clearTimeout(timer);
    running = true;
    lastStarted = Date.now();
    try {
      await load();
      failures = 0;
    } catch {
      // The view owns the error message; transient failures must not stop polling.
      failures = Math.min(failures + 1, 4);
    } finally {
      running = false;
      schedule(Math.min(interval * 2 ** failures, 60000));
    }
  }
  function wake() {
    if (!available()) {
      clearTimeout(timer);
      return;
    }
    if (!running) schedule(Math.max(0, 500 - (Date.now() - lastStarted)));
  }
  function stop() {
    clearTimeout(timer);
    document.removeEventListener("visibilitychange", wake);
    window.removeEventListener("focus", wake);
    window.removeEventListener("online", wake);
    window.removeEventListener("offline", wake);
    signal.removeEventListener("abort", stop);
  }
  if (signal.aborted) return;
  document.addEventListener("visibilitychange", wake);
  window.addEventListener("focus", wake);
  window.addEventListener("online", wake);
  window.addEventListener("offline", wake);
  signal.addEventListener("abort", stop, { once: true });
  if (immediate) void run(true);
  else schedule(interval);
}

export const unchanged = <T>(previous: T, next: T): T =>
  JSON.stringify(previous) === JSON.stringify(next) ? previous : next;

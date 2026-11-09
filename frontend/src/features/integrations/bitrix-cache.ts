import type { RemotePage } from "../../shared/types";
import { api } from "../../shared/api";

// Session-only LRU. Never persist private conversations in browser storage.
export class DialogCache {
  private entries = new Map<
    string,
    { value: RemotePage; at: number; bytes: number }
  >();
  private bytes = 0;
  constructor(
    private maxPages = 24,
    private maxBytes = 4_000_000,
    private ttl = 300_000,
  ) {}
  get(account: string, dialog: string, cursor: string): RemotePage | undefined {
    if (!account) return;
    const key = JSON.stringify([account, dialog, cursor]);
    const entry = this.entries.get(key);
    if (!entry) return;
    this.entries.delete(key);
    if (Date.now() - entry.at > this.ttl) {
      this.bytes -= entry.bytes;
      return;
    }
    this.entries.set(key, entry);
    return entry.value;
  }
  put(account: string, dialog: string, cursor: string, value: RemotePage) {
    if (!account || value.accountId !== account) return;
    const key = JSON.stringify([account, dialog, cursor]);
    const previous = this.entries.get(key);
    if (previous) this.bytes -= previous.bytes;
    this.entries.delete(key);
    const bytes = JSON.stringify(value).length * 2;
    if (bytes > this.maxBytes) return;
    this.entries.set(key, { value, bytes, at: Date.now() });
    this.bytes += bytes;
    while (this.entries.size > this.maxPages || this.bytes > this.maxBytes) {
      const oldest = this.entries.keys().next().value!;
      this.bytes -= this.entries.get(oldest)!.bytes;
      this.entries.delete(oldest);
    }
  }
}

export async function bitrixPage(
  path: string,
  signal: AbortSignal,
): Promise<RemotePage> {
  const abort = new AbortController();
  const cancel = () => abort.abort();
  signal.addEventListener("abort", cancel, { once: true });
  if (signal.aborted) abort.abort();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    abort.abort();
  }, 12_000);
  try {
    return await api<RemotePage>(path, "GET", undefined, abort.signal);
  } catch (e) {
    if (timedOut)
      throw new Error("Битрикс24 долго отвечает. Попробуйте обновить диалог.");
    throw e;
  } finally {
    clearTimeout(timer);
    signal.removeEventListener("abort", cancel);
  }
}

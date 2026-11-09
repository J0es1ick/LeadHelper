import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api } from "../shared/api";
import { poll, unchanged } from "../shared/live";
import type { State } from "../shared/types";
type Context = {
  state: State | null;
  loading: boolean;
  error: string;
  refresh: () => Promise<void>;
  act: <T>(fn: () => Promise<T>, message?: string) => Promise<T>;
  notify: (text: string, error?: boolean) => void;
  toast: { text: string; error: boolean } | null;
};
const Store = createContext<Context | null>(null);
export function Provider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<State | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [toast, setToast] = useState<Context["toast"]>(null);
  const refreshVersion = useRef(0);
  const notify = useCallback(
    (text: string, error = false) => setToast({ text, error }),
    [],
  );
  const refresh = useCallback(async (signal?: AbortSignal) => {
    const version = ++refreshVersion.current;
    try {
      const value = await api<State>("/state", "GET", undefined, signal);
      if (!signal?.aborted && version === refreshVersion.current) {
        setState((old) => unchanged(old, value));
        setError("");
      }
    } catch (e) {
      if (!signal?.aborted && version === refreshVersion.current)
        setError((e as Error).message);
      throw e;
    } finally {
      if (!signal?.aborted && version === refreshVersion.current)
        setLoading(false);
    }
  }, []);
  useEffect(() => {
    const c = new AbortController();
    poll(() => refresh(c.signal), {
      signal: c.signal,
      interval: 5000,
      background: true,
    });
    return () => c.abort();
  }, [refresh]);
  useEffect(() => {
    if (toast) {
      const t = setTimeout(() => setToast(null), 4500);
      return () => clearTimeout(t);
    }
  }, [toast]);
  async function act<T>(fn: () => Promise<T>, message?: string) {
    try {
      const result = await fn();
      await refresh();
      if (message) notify(message);
      return result;
    } catch (e) {
      notify((e as Error).message, true);
      throw e;
    }
  }
  return (
    <Store.Provider
      value={{ state, loading, error, refresh, act, notify, toast }}
    >
      {children}
    </Store.Provider>
  );
}
export function useStore() {
  const value = useContext(Store);
  if (!value) throw Error("Provider missing");
  return value;
}
export function useData() {
  const { state, ...rest } = useStore();
  if (!state) throw Error("State not loaded");
  return { state, ...rest };
}

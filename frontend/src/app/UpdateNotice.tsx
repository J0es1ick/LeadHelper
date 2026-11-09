import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
export function UpdateNotice() {
  const [available, setAvailable] = useState(false);
  useEffect(() => {
    const current = document
      .querySelector<HTMLScriptElement>(
        'script[type="module"][src*="/assets/"]',
      )
      ?.getAttribute("src");
    if (!current) return;
    const controller = new AbortController();
    const check = async () => {
      if (document.hidden) return;
      try {
        const response = await fetch("/api/health", {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!response.ok) return;
        const info = (await response.json()) as { frontendBuild?: string };
        if (
          !controller.signal.aborted &&
          info.frontendBuild &&
          info.frontendBuild !== current
        )
          setAvailable(true);
      } catch {
        /* The normal connection indicator handles outages. */
      }
    };
    void check();
    const interval = window.setInterval(() => void check(), 30000);
    window.addEventListener("focus", check);
    return () => {
      controller.abort();
      clearInterval(interval);
      window.removeEventListener("focus", check);
    };
  }, []);
  if (!available) return null;
  return (
    <div className="app-update-notice" role="status">
      <RefreshCw size={18} />
      <span>
        <strong>Доступно обновление LeadHelper</strong>
        <small>Сохраните черновики перед обновлением страницы.</small>
      </span>
      <button className="button small" onClick={() => location.reload()}>
        Обновить страницу
      </button>
    </div>
  );
}

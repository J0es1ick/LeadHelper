import { useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, ZoomIn, ZoomOut } from "lucide-react";

export default function DocxPreview({
  url,
  fallback,
}: {
  url: string;
  fallback: string;
}) {
  const frame = useRef<HTMLIFrameElement>(null);
  const completed = useRef(false);
  const [ready, setReady] = useState(false),
    [busy, setBusy] = useState(true),
    [error, setError] = useState(""),
    [text, setText] = useState(false);
  const [pages, setPages] = useState(0),
    [page, setPage] = useState(0),
    [zoom, setZoom] = useState(100);
  useEffect(() => {
    const receive = (e: MessageEvent) => {
      if (e.source !== frame.current?.contentWindow) return;
      if (e.data?.type === "leadhelper:docx:ready") setReady(true);
      if (e.data?.type === "leadhelper:docx:rendered") {
        completed.current = true;
        setPages(e.data.pages);
        setZoom(e.data.zoom);
        setBusy(false);
      }
      if (e.data?.type === "leadhelper:docx:error") {
        completed.current = true;
        setError(e.data.message);
        setBusy(false);
      }
    };
    window.addEventListener("message", receive);
    return () => window.removeEventListener("message", receive);
  }, []);
  useEffect(() => {
    const timer = setTimeout(() => {
      if (!completed.current) {
        setError(
          "Документ обрабатывается слишком долго. Можно открыть текст или скачать оригинал.",
        );
        setBusy(false);
      }
    }, 30000);
    return () => clearTimeout(timer);
  }, [url]);
  useEffect(() => {
    if (!ready) return;
    const abort = new AbortController();
    const content = new URL(url, location.origin);
    content.searchParams.set("format", "docx");
    void fetch(content, { signal: abort.signal })
      .then(async (r) => {
        if (!r.ok) throw Error("Не удалось загрузить документ");
        return r.arrayBuffer();
      })
      .then((bytes) => {
        if (!abort.signal.aborted)
          frame.current?.contentWindow?.postMessage(
            { type: "leadhelper:docx:load", bytes },
            "*",
            [bytes],
          );
      })
      .catch((e) => {
        if (!abort.signal.aborted) {
          completed.current = true;
          setError(e.message);
          setBusy(false);
        }
      });
    return () => {
      abort.abort();
    };
  }, [ready, url]);
  useEffect(() => {
    if (!busy)
      frame.current?.contentWindow?.postMessage(
        { type: "leadhelper:docx:view", page, zoom },
        "*",
      );
  }, [page, zoom, busy]);
  return (
    <div className="office-preview">
      <div className="office-controls">
        <div className="segmented">
          <button
            className={!text ? "active" : ""}
            onClick={() => setText(false)}
          >
            Документ
          </button>
          <button
            className={text ? "active" : ""}
            onClick={() => setText(true)}
          >
            Текст
          </button>
        </div>
        {!text && pages > 0 && (
          <>
            <button
              className="icon-button"
              aria-label="Предыдущая страница документа"
              disabled={page === 0}
              onClick={() => setPage((v) => v - 1)}
            >
              <ChevronLeft size={18} />
            </button>
            <span>
              Страница {page + 1} из {pages}
            </span>
            <button
              className="icon-button"
              aria-label="Следующая страница документа"
              disabled={page + 1 >= pages}
              onClick={() => setPage((v) => v + 1)}
            >
              <ChevronRight size={18} />
            </button>
            <span className="spacer" />
            <button
              className="icon-button"
              aria-label="Уменьшить документ"
              disabled={zoom <= 25}
              onClick={() => setZoom((v) => Math.max(25, v - 10))}
            >
              <ZoomOut size={18} />
            </button>
            <output>{zoom}%</output>
            <button
              className="icon-button"
              aria-label="Увеличить документ"
              disabled={zoom >= 200}
              onClick={() => setZoom((v) => Math.min(200, v + 10))}
            >
              <ZoomIn size={18} />
            </button>
          </>
        )}
      </div>
      {!text && busy && (
        <p role="status" className="file-preview-notice">
          Восстанавливаем оформление документа…
        </p>
      )}
      {!text && error && (
        <p role="alert" className="file-preview-notice">
          {error}
        </p>
      )}
      <iframe
        ref={frame}
        title="Документ Word"
        className="office-document-frame"
        style={{ display: text ? "none" : undefined }}
        sandbox="allow-scripts"
        src="/office/viewer.html"
        onLoad={() =>
          frame.current?.contentWindow?.postMessage(
            { type: "leadhelper:docx:hello" },
            "*",
          )
        }
      />
      {text && (
        <pre className="file-preview-text" tabIndex={0}>
          {fallback || "Текст отсутствует."}
        </pre>
      )}
    </div>
  );
}

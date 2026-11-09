import { useEffect, useRef, useState } from "react";
import {
  ChevronLeft,
  ChevronRight,
  LoaderCircle,
  ZoomIn,
  ZoomOut,
} from "lucide-react";
import {
  getDocument,
  GlobalWorkerOptions,
  type PDFDocumentProxy,
  type RenderTask,
} from "../../vendor/pdfjs/pdf.mjs";

GlobalWorkerOptions.workerSrc = new URL(
  "../../vendor/pdfjs/pdf.worker.min.mjs",
  import.meta.url,
).href;

export default function PdfPreview({ url }: { url: string }) {
  const [document, setDocument] = useState<PDFDocumentProxy | null>(null);
  const [pageNumber, setPageNumber] = useState(1);
  const [zoom, setZoom] = useState(100);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const canvas = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    let cancelled = false;
    const task = getDocument({
      url,
      cMapUrl: "/vendor/pdfjs/cmaps/",
      cMapPacked: true,
      standardFontDataUrl: "/vendor/pdfjs/standard_fonts/",
      wasmUrl: "/vendor/pdfjs/wasm/",
      iccUrl: "/vendor/pdfjs/iccs/",
      isEvalSupported: false,
      enableXfa: false,
      maxImageSize: 16_000_000,
      canvasMaxAreaInBytes: 64_000_000,
    });
    void task.promise
      .then((pdf) => {
        if (!cancelled) setDocument(pdf);
      })
      .catch((e: Error) => {
        if (!cancelled) {
          console.warn("PDF preview could not load:", e.name, e.message);
          setError(
            e.name === "PasswordException"
              ? "PDF защищён паролем. Скачайте оригинал и откройте его в программе для чтения PDF."
              : "Не удалось прочитать PDF. Файл может быть повреждён; попробуйте открыть оригинал.",
          );
          setBusy(false);
        }
      });
    return () => {
      cancelled = true;
      void task.destroy().catch(() => {});
    };
  }, [url]);

  useEffect(() => {
    if (!document) return;
    let cancelled = false;
    let render: RenderTask | undefined;
    setBusy(true);
    setError("");
    void document
      .getPage(pageNumber)
      .then(async (page) => {
        const target = canvas.current;
        if (cancelled || !target) return;
        const original = page.getViewport({ scale: 1 });
        const scale = Math.min(
          (2 * zoom) / 100,
          Math.sqrt(16_000_000 / (original.width * original.height)),
        );
        const viewport = page.getViewport({ scale });
        target.width = Math.ceil(viewport.width);
        target.height = Math.ceil(viewport.height);
        target.style.width = `${zoom}%`;
        render = page.render({ canvas: target, viewport });
        await render.promise;
        if (!cancelled) setBusy(false);
      })
      .catch((e: Error) => {
        if (!cancelled && e.name !== "RenderingCancelledException") {
          setError(
            "Не удалось отобразить страницу PDF. Попробуйте скачать оригинал.",
          );
          setBusy(false);
        }
      });
    return () => {
      cancelled = true;
      render?.cancel();
    };
  }, [document, pageNumber, zoom]);

  return (
    <div className="pdf-viewer">
      <div className="pdf-controls">
        <button
          className="icon-button"
          aria-label="Предыдущая страница"
          disabled={pageNumber <= 1 || busy}
          onClick={() => setPageNumber((v) => v - 1)}
        >
          <ChevronLeft size={18} />
        </button>
        <span aria-live="polite">
          Страница {pageNumber} из {document?.numPages ?? "…"}
        </span>
        <button
          className="icon-button"
          aria-label="Следующая страница"
          disabled={!document || pageNumber >= document.numPages || busy}
          onClick={() => setPageNumber((v) => v + 1)}
        >
          <ChevronRight size={18} />
        </button>
        <span className="spacer" />
        <button
          className="icon-button"
          aria-label="Уменьшить PDF"
          disabled={zoom <= 50 || busy}
          onClick={() => setZoom((v) => v - 25)}
        >
          <ZoomOut size={17} />
        </button>
        <output>{zoom}%</output>
        <button
          className="icon-button"
          aria-label="Увеличить PDF"
          disabled={zoom >= 200 || busy}
          onClick={() => setZoom((v) => v + 25)}
        >
          <ZoomIn size={17} />
        </button>
      </div>
      {error && (
        <p className="file-preview-notice" role="alert">
          {error}
        </p>
      )}
      {busy && !error && (
        <p className="pdf-loading" role="status">
          <LoaderCircle size={18} className="spin" />
          Готовим страницу…
        </p>
      )}
      <div className="pdf-pages" aria-busy={busy} hidden={!!error}>
        <canvas
          ref={canvas}
          role="img"
          aria-label={`Страница ${pageNumber} документа PDF`}
          style={{ visibility: busy ? "hidden" : "visible" }}
        />
      </div>
    </div>
  );
}

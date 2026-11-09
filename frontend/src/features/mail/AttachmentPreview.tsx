import { lazy, Suspense, useEffect, useState } from "react";
import {
  Download,
  FileText,
  LoaderCircle,
  RotateCw,
  ZoomIn,
  ZoomOut,
} from "lucide-react";
import { endpoint } from "../../shared/api";
import { Modal } from "../../shared/ui";
import type { Attachment } from "../../shared/types";
import "./attachment-preview.css";
import type { SpreadsheetData } from "./SheetPreview";

const PdfPreview = lazy(() => import("./PdfPreview"));
const DocxPreview = lazy(() => import("./DocxPreview"));
const SheetPreview = lazy(() => import("./SheetPreview"));

interface Preview extends SpreadsheetData {
  kind: "image" | "pdf" | "docx" | "text" | "table" | "unsupported";
  text?: string;
  notice?: string;
  truncated: boolean;
}

export function attachmentURL(messageId: string, attachmentId: string) {
  return (
    "/api/mail/" +
    endpoint(messageId) +
    "/attachments/" +
    endpoint(attachmentId)
  );
}

export function AttachmentPreview({
  messageId,
  file,
  onClose,
}: {
  messageId: string;
  file: Attachment;
  onClose: () => void;
}) {
  const url = attachmentURL(messageId, file.id);
  return (
    <FilePreview
      file={{ name: file.name, size: file.size ?? file.data.length * 0.75 }}
      downloadURL={url}
      previewURL={url + "/preview"}
      contentURL={url + "/preview/content"}
      onClose={onClose}
    />
  );
}

export function FilePreview({
  file,
  downloadURL,
  previewURL,
  contentURL,
  onClose,
  onDownload,
}: {
  file: { name: string; size: number };
  downloadURL: string;
  previewURL: string;
  contentURL: string;
  onClose: () => void;
  onDownload?: () => Promise<void>;
}) {
  const [preview, setPreview] = useState<Preview | null>(null);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [zoom, setZoom] = useState(100);
  const [downloading, setDownloading] = useState(false),
    [downloadError, setDownloadError] = useState("");
  async function download() {
    if (!onDownload || downloading) return;
    setDownloading(true);
    setDownloadError("");
    try {
      await onDownload();
    } catch (e) {
      setDownloadError((e as Error).message);
    } finally {
      setDownloading(false);
    }
  }

  useEffect(() => {
    const controller = new AbortController();
    setPreview(null);
    setError("");
    setZoom(100);
    void fetch(previewURL, { signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) {
          const body = await response.json().catch(() => ({}));
          throw Error(body.error || "Не удалось загрузить файл");
        }
        return response.json() as Promise<Preview>;
      })
      .then(setPreview)
      .catch((e: Error) => {
        if (!controller.signal.aborted) setError(e.message);
      });
    return () => controller.abort();
  }, [previewURL, attempt]);

  return (
    <Modal title={file.name} onClose={onClose} wide>
      <div className="file-preview">
        <div className="file-preview-toolbar">
          <span>{formatFileSize(file.size)}</span>
          <span className="spacer" />
          {preview?.kind === "image" && !error && (
            <div className="file-preview-zoom">
              <button
                className="icon-button"
                aria-label="Уменьшить изображение"
                disabled={zoom <= 50}
                onClick={() => setZoom((v) => v - 25)}
              >
                <ZoomOut size={17} />
              </button>
              <output aria-live="polite">{zoom}%</output>
              <button
                className="icon-button"
                aria-label="Увеличить изображение"
                disabled={zoom >= 200}
                onClick={() => setZoom((v) => v + 25)}
              >
                <ZoomIn size={17} />
              </button>
            </div>
          )}
          {onDownload ? (
            <button
              className="button"
              disabled={downloading}
              onClick={() => void download()}
            >
              <Download size={16} />
              {downloading ? "Скачивание…" : "Скачать оригинал"}
            </button>
          ) : (
            <a className="button" href={downloadURL} download={file.name}>
              <Download size={16} />
              Скачать оригинал
            </a>
          )}
        </div>
        {downloadError && (
          <p className="file-preview-notice" role="alert">
            {downloadError}
          </p>
        )}
        {preview?.notice && preview.kind !== "unsupported" && (
          <p className="file-preview-notice">{preview.notice}</p>
        )}
        {preview?.truncated && (
          <p className="file-preview-notice">
            Показана часть файла. Полное содержимое доступно в оригинале.
          </p>
        )}
        {error ? (
          <div className="file-preview-empty" role="alert">
            <FileText size={32} />
            <strong>Не удалось открыть предпросмотр</strong>
            <p>{error}</p>
            <button className="button" onClick={() => setAttempt((v) => v + 1)}>
              <RotateCw size={16} />
              Повторить
            </button>
          </div>
        ) : !preview ? (
          <div className="file-preview-empty" role="status">
            <LoaderCircle size={28} className="spin" />
            Загружаем предпросмотр…
          </div>
        ) : (
          <>
            {preview.kind === "image" && (
              <div className="file-preview-image">
                <img
                  key={attempt}
                  src={contentURL}
                  alt={file.name}
                  style={{ width: `${zoom}%` }}
                  onError={() =>
                    setError(
                      "Браузер не смог прочитать изображение. Попробуйте скачать оригинал.",
                    )
                  }
                />
              </div>
            )}
            {preview.kind === "pdf" && (
              <Suspense
                fallback={
                  <div className="file-preview-empty" role="status">
                    Загружаем просмотрщик PDF…
                  </div>
                }
              >
                <PdfPreview url={contentURL} />
              </Suspense>
            )}
            {preview.kind === "text" && (
              <pre className="file-preview-text" tabIndex={0}>
                {preview.text || "Файл не содержит текста."}
              </pre>
            )}
            {preview.kind === "docx" && (
              <Suspense
                fallback={<p role="status">Загружаем просмотрщик Word…</p>}
              >
                <DocxPreview url={contentURL} fallback={preview.text || ""} />
              </Suspense>
            )}
            {preview.kind === "table" && (
              <Suspense fallback={<p role="status">Загружаем таблицу…</p>}>
                <SheetPreview sheets={preview.sheets} styles={preview.styles} />
              </Suspense>
            )}
            {preview.kind === "unsupported" && (
              <div className="file-preview-empty">
                <FileText size={36} />
                <strong>Предпросмотр недоступен</strong>
                <p>{preview.notice}</p>
              </div>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}

export function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${Math.round(bytes)} Б`;
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)} КБ`;
  return `${(bytes / (1024 * 1024)).toLocaleString("ru-RU", { maximumFractionDigits: 1 })} МБ`;
}

import { useState } from "react";
import { Download, FileText, Eye } from "lucide-react";
import type { RemoteAttachment } from "../../shared/types";
import { BitrixFilePreview, downloadBitrixFile } from "./BitrixFilePreview";
import "./chat-attachments.css";

export function ChatAttachments({
  files = [],
}: {
  files?: RemoteAttachment[];
}) {
  const [preview, setPreview] = useState<RemoteAttachment | null>(null);
  const [broken, setBroken] = useState<Record<string, boolean>>({});
  const [error, setError] = useState(""),
    [busy, setBusy] = useState("");
  async function download(file: RemoteAttachment) {
    if (busy) return;
    setBusy(file.id);
    setError("");
    try {
      await downloadBitrixFile(file);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy("");
    }
  }
  if (!files.length) return null;
  return (
    <div className="chat-attachments">
      {files.map((file) => (
        <div className="chat-attachment" key={file.id}>
          {file.image && !broken[file.id] ? (
            <button
              className="chat-image-button"
              aria-label={"Открыть изображение " + file.name}
              onClick={() => setPreview(file)}
            >
              <img
                loading="lazy"
                src={file.url + "&preview=1"}
                alt={file.name}
                onError={() =>
                  setBroken((old) => ({ ...old, [file.id]: true }))
                }
              />
            </button>
          ) : (
            <FileText size={25} />
          )}
          <div className="chat-file-meta">
            <button
              className="chat-file-name"
              title={file.name}
              onClick={() => setPreview(file)}
            >
              {file.name}
            </button>
            <small>
              {file.size
                ? file.size >= 1048576
                  ? (file.size / 1048576).toFixed(1) + " МБ"
                  : Math.max(1, Math.round(file.size / 1024)) + " КБ"
                : "Файл"}
              {broken[file.id] ? " · предпросмотр недоступен" : ""}
            </small>
          </div>
          <button
            className="icon-button"
            aria-label={"Предпросмотр " + file.name}
            title="Предпросмотр"
            onClick={() => setPreview(file)}
          >
            <Eye size={18} />
          </button>
          <button
            className="icon-button"
            disabled={!!busy}
            aria-label={"Скачать " + file.name}
            onClick={() => void download(file)}
          >
            <Download size={18} />
          </button>
        </div>
      ))}
      {busy && <small role="status">Загрузка файла…</small>}
      {error && (
        <p className="inline-warning" role="alert">
          {error}
        </p>
      )}
      {preview && (
        <BitrixFilePreview file={preview} onClose={() => setPreview(null)} />
      )}
    </div>
  );
}

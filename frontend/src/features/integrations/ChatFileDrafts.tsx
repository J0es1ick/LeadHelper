import { useEffect, useState } from "react";
import { FileText, X } from "lucide-react";
import type { ChatFileDraft } from "./chat-upload";
function DraftImage({ file }: { file: File }) {
  const [url, setURL] = useState("");
  useEffect(() => {
    if (!/^image\/(png|jpeg|gif|webp|bmp)$/i.test(file.type)) return;
    const value = URL.createObjectURL(file);
    setURL(value);
    return () => URL.revokeObjectURL(value);
  }, [file]);
  return url ? <img src={url} alt={file.name} /> : <FileText size={22} />;
}
export function ChatFileDrafts({
  items,
  disabled,
  remove,
}: {
  items: ChatFileDraft[];
  disabled: boolean;
  remove: (id: string) => void;
}) {
  return (
    <div className="bx-file-drafts" aria-label="Вложения к сообщению">
      {items.map((item) => (
        <div className={"bx-file-draft " + item.state} key={item.id}>
          <div className="bx-file-thumbnail">
            <DraftImage file={item.file} />
          </div>
          <div>
            <strong title={item.file.name}>{item.file.name}</strong>
            <small>
              {item.file.size < 1024 * 1024
                ? `${Math.ceil(item.file.size / 1024)} КБ`
                : `${(item.file.size / 1024 / 1024).toFixed(1)} МБ`}
              {item.state === "sending"
                ? item.progress < 100
                  ? ` · Загрузка ${item.progress}%`
                  : " · Отправляем…"
                : item.state === "uncertain"
                  ? " · Проверьте отправку в чате"
                  : ""}
            </small>
            {item.state === "sending" && (
              <progress
                max={100}
                value={item.progress}
                aria-label={"Загрузка " + item.file.name}
              />
            )}
          </div>
          <button
            type="button"
            className="icon-button"
            aria-label={"Убрать " + item.file.name}
            disabled={disabled}
            onClick={() => remove(item.id)}
          >
            <X size={15} />
          </button>
        </div>
      ))}
    </div>
  );
}

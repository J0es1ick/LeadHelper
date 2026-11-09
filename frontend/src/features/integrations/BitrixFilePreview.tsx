import type { RemoteAttachment } from "../../shared/types";
import { FilePreview } from "../mail/AttachmentPreview";

export async function downloadBitrixFile(file: RemoteAttachment) {
  const response = await fetch(file.url);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data.error || "Не удалось скачать файл");
  }
  const url = URL.createObjectURL(await response.blob());
  const a = document.createElement("a");
  a.href = url;
  a.download = file.name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function BitrixFilePreview({
  file,
  onClose,
}: {
  file: RemoteAttachment;
  onClose: () => void;
}) {
  const preview = new URL(file.url, window.location.origin);
  preview.pathname += "/preview";
  const content = new URL(preview);
  content.pathname += "/content";
  return (
    <FilePreview
      file={file}
      downloadURL={file.url}
      previewURL={preview.pathname + preview.search}
      contentURL={content.pathname + content.search}
      onClose={onClose}
      onDownload={() => downloadBitrixFile(file)}
    />
  );
}

import { endpoint } from "../../shared/api";
export type ChatFileDraft = {
  id: string;
  file: File;
  state: "pending" | "sending" | "uncertain";
  progress: number;
};
export type FileDraftUpdate = (previous: ChatFileDraft[]) => ChatFileDraft[];
export const chatFileLimit = 25 * 1024 * 1024;
export function addChatFiles(previous: ChatFileDraft[], incoming: File[]) {
  const items = [...previous],
    errors: string[] = [];
  let total = items.reduce((sum, item) => sum + item.file.size, 0);
  for (const file of incoming) {
    if (!file.size) {
      errors.push(`${file.name}: файл пустой`);
      continue;
    }
    if (file.size > chatFileLimit) {
      errors.push(`${file.name}: больше 25 МБ`);
      continue;
    }
    if (
      items.some(
        (x) =>
          x.file.name === file.name &&
          x.file.size === file.size &&
          x.file.lastModified === file.lastModified,
      )
    )
      continue;
    if (items.length >= 10 || total + file.size > 100 * 1024 * 1024) {
      errors.push("Можно прикрепить до 10 файлов общим размером до 100 МБ");
      break;
    }
    items.push({
      id: crypto.randomUUID(),
      file,
      state: "pending",
      progress: 0,
    });
    total += file.size;
  }
  return { items, error: errors.join(". ") };
}
export class ChatUploadError extends Error {
  constructor(
    message: string,
    public uncertain: boolean,
  ) {
    super(message);
  }
}
export function uploadChatFile(
  dialog: string,
  account: string,
  item: ChatFileDraft,
  text: string,
  progress: (value: number) => void,
) {
  return new Promise<void>((resolve, reject) => {
    const request = new XMLHttpRequest(),
      form = new FormData();
    form.append("file", item.file);
    form.append("text", text);
    form.append("accountId", account);
    form.append("requestId", item.id);
    request.open("POST", `/api/bitrix/chats/${endpoint(dialog)}/files`);
    request.setRequestHeader("X-LeadHelper", "1");
    request.timeout = 105000;
    request.upload.onprogress = (e) => {
      if (e.lengthComputable) progress(Math.round((e.loaded / e.total) * 100));
    };
    request.onload = () => {
      let result: { messageId?: string; error?: string; uncertain?: boolean } =
        {};
      try {
        result = JSON.parse(request.responseText);
      } catch {}
      if (request.status >= 200 && request.status < 300 && result.messageId)
        resolve();
      else
        reject(
          new ChatUploadError(
            result.error || "Не удалось подтвердить отправку файла",
            result.uncertain ?? (request.status >= 500 || request.status < 400),
          ),
        );
    };
    request.onerror =
      request.ontimeout =
      request.onabort =
        () =>
          reject(
            new ChatUploadError(
              "Соединение прервалось. Проверьте чат перед повторной отправкой",
              true,
            ),
          );
    request.send(form);
  });
}

import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  Bold,
  Italic,
  Underline,
  List,
  ListOrdered,
  Link,
  ImagePlus,
  Paperclip,
  Send,
  X,
  Undo,
  Redo,
  RemoveFormatting,
} from "lucide-react";
import { useData } from "../../app/store";
import { api, endpoint } from "../../shared/api";
import { Modal } from "../../shared/ui";
import type { Attachment, Message } from "../../shared/types";
import { cleanHTML, plainHTML, resolveInline } from "./html";

export function Composer({
  initial,
  onClose,
  sourceId,
}: {
  initial: Partial<Message>;
  sourceId?: string;
  onClose: () => void;
}) {
  const { state, act, notify } = useData();
  const [form, setForm] = useState<Partial<Message>>({
    ...initial,
    to: initial.to || "",
    cc: initial.cc || "",
    subject: initial.subject || "",
    body: initial.body || "",
    attachments: initial.attachments || [],
  });
  const [busy, setBusy] = useState(false),
    [savedId, setSavedId] = useState(initial.id || ""),
    [link, setLink] = useState<string | null>(null);
  const editor = useRef<HTMLDivElement>(null),
    files = useRef<HTMLInputElement>(null),
    images = useRef<HTMLInputElement>(null),
    range = useRef<Range | null>(null);
  const attachments = useRef(form.attachments || []);
  const initialSource = useRef(initial);
  const smtpMissing =
    !state.settings.demo && !state.mailConnection?.sendConfigured;
  function imageSource(src: string, files: Attachment[]) {
    const file = files.find((a) => "cid:" + a.contentId === src);
    const owner =
      sourceId ||
      initial.id ||
      state.messages.find((m) =>
        m.attachments?.some(
          (a) => a.id === file?.id && a.blobKey === file?.blobKey,
        ),
      )?.id;
    return resolveInline(src, files, owner);
  }
  useEffect(() => {
    const value = initialSource.current;
    const el = editor.current;
    if (!el) return;
    el.innerHTML = cleanHTML(
      value.html || plainHTML(value.body || ""),
      (src) =>
        src.startsWith("cid:") && imageSource(src, value.attachments || [])
          ? src
          : null,
    );
    el.querySelectorAll("img").forEach((img) => {
      const cid = img.getAttribute("src")!;
      img.dataset.contentId = cid.slice(4);
      img.src = imageSource(cid, value.attachments || [])!;
    });
  }, []);
  function remember() {
    const s = window.getSelection();
    if (s?.rangeCount && editor.current?.contains(s.anchorNode))
      range.current = s.getRangeAt(0).cloneRange();
  }
  function restore() {
    editor.current?.focus();
    const s = window.getSelection();
    if (
      range.current &&
      editor.current?.contains(range.current.commonAncestorContainer)
    ) {
      s?.removeAllRanges();
      s?.addRange(range.current);
    } else if (editor.current) {
      const r = document.createRange();
      r.selectNodeContents(editor.current);
      r.collapse(false);
      s?.removeAllRanges();
      s?.addRange(r);
    }
  }
  function command(name: string, value?: string) {
    restore();
    document.execCommand(name, false, value);
    remember();
  }
  function content() {
    const el = editor.current!;
    const copy = el.cloneNode(true) as HTMLElement;
    copy.querySelectorAll("img").forEach((img) => {
      const a = attachments.current.find(
        (a) => a.contentId === img.dataset.contentId,
      );
      if (a) img.setAttribute("src", "cid:" + a.contentId);
      else img.remove();
    });
    const html = cleanHTML(copy.innerHTML, (src) =>
      src.startsWith("cid:") &&
      attachments.current.some((a) => "cid:" + a.contentId === src)
        ? src
        : null,
    );
    return {
      html,
      body:
        el.innerText.trim() ||
        (copy.querySelector("img") ? "[Изображение]" : ""),
    };
  }
  async function save(send = false) {
    if (busy) return;
    setBusy(true);
    try {
      const draft = await act(() =>
        api<Message>(
          "/mail/drafts" + (savedId ? "/" + endpoint(savedId) : ""),
          savedId ? "PUT" : "POST",
          { ...form, ...content(), attachments: attachments.current },
        ),
      );
      setSavedId(draft.id);
      if (send) {
        await act(
          () => api("/mail/" + endpoint(draft.id) + "/send", "POST"),
          state.settings.demo
            ? "Тестовое письмо сохранено в отправленных"
            : "Письмо принято провайдером",
        );
      } else notify("Черновик сохранён");
      onClose();
    } catch {
      /* Keep editor and saved ID for retry. */
    } finally {
      setBusy(false);
    }
  }
  async function attach(list: File[], inline = false) {
    if (!list.length) return;
    const old = attachments.current;
    if (
      old.length + list.length > 10 ||
      old.reduce((n, a) => n + (a.size ?? a.data.length * 0.75), 0) +
        list.reduce((n, f) => n + f.size, 0) >
        5 * 1024 * 1024
    ) {
      notify("Не более 10 вложений общим размером до 5 МБ", true);
      return;
    }
    if (
      inline &&
      list.some((f) => !/^image\/(png|jpeg|gif|webp|bmp)$/.test(f.type))
    ) {
      notify("Для вставки в текст выберите PNG, JPEG, GIF, WebP или BMP", true);
      return;
    }
    setBusy(true);
    try {
      const added: Attachment[] = [];
      for (const f of list) {
        const data = await new Promise<string>((resolve, reject) => {
          const r = new FileReader();
          r.onload = () => resolve(String(r.result).split(",")[1]);
          r.onerror = reject;
          r.readAsDataURL(f);
        });
        const id = crypto.randomUUID();
        added.push({
          id,
          name: f.name || "image.png",
          contentType: f.type || "application/octet-stream",
          data,
          size: f.size,
          inline,
          contentId: inline ? id + "@leadhelper" : undefined,
        });
      }
      attachments.current = [...attachments.current, ...added];
      setForm((f) => ({ ...f, attachments: attachments.current }));
      if (inline)
        for (const a of added) {
          const before = new Set(editor.current?.querySelectorAll("img"));
          command("insertImage", resolveInline("cid:" + a.contentId, [a])!);
          editor.current?.querySelectorAll("img").forEach((img) => {
            if (!before.has(img)) img.dataset.contentId = a.contentId;
          });
        }
    } catch {
      notify("Не удалось прочитать файл", true);
    } finally {
      setBusy(false);
    }
  }
  function remove(a: Attachment) {
    editor.current?.querySelectorAll("img").forEach((img) => {
      if (img.dataset.contentId === a.contentId) img.remove();
    });
    attachments.current = attachments.current.filter((v) => v.id !== a.id);
    setForm((f) => ({ ...f, attachments: attachments.current }));
  }
  const tools = [
    { label: "Жирный", command: "bold", icon: Bold },
    { label: "Курсив", command: "italic", icon: Italic },
    { label: "Подчёркнутый", command: "underline", icon: Underline },
    {
      label: "Маркированный список",
      command: "insertUnorderedList",
      icon: List,
    },
    {
      label: "Нумерованный список",
      command: "insertOrderedList",
      icon: ListOrdered,
    },
    {
      label: "Убрать форматирование",
      command: "removeFormat",
      icon: RemoveFormatting,
    },
    { label: "Отменить", command: "undo", icon: Undo },
    { label: "Повторить", command: "redo", icon: Redo },
  ];
  return (
    <Modal
      title={initial.id ? "Редактировать черновик" : "Новое письмо"}
      onClose={onClose}
      wide
    >
      <form
        className="form compose-form"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          void save(true);
        }}
      >
        <p className="compose-from">
          От: {state.mailConnection?.email || state.settings.email}
        </p>
        <label>
          Кому
          <input
            aria-label="Кому"
            required
            value={form.to}
            onChange={(e) => setForm({ ...form, to: e.target.value })}
            placeholder="name@company.ru, colleague@company.ru"
          />
        </label>
        <label>
          Копия
          <input
            value={form.cc}
            onChange={(e) => setForm({ ...form, cc: e.target.value })}
            placeholder="Необязательно"
          />
        </label>
        <label>
          Тема
          <input
            aria-label="Тема"
            required
            maxLength={300}
            value={form.subject}
            onChange={(e) => setForm({ ...form, subject: e.target.value })}
            placeholder="О чём письмо?"
          />
        </label>
        <div className="rich-editor">
          <div
            className="editor-toolbar"
            role="toolbar"
            aria-label="Форматирование письма"
          >
            {tools.map((t) => (
              <button
                type="button"
                className="icon-button"
                key={t.command}
                title={t.label}
                aria-label={t.label}
                disabled={busy}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => command(t.command)}
              >
                <t.icon size={17} />
              </button>
            ))}
            <button
              type="button"
              className="icon-button"
              title="Добавить ссылку"
              aria-label="Добавить ссылку"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => {
                remember();
                setLink("");
              }}
            >
              <Link size={17} />
            </button>
            <button
              type="button"
              className="button editor-image"
              disabled={busy}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => {
                remember();
                images.current?.click();
              }}
            >
              <ImagePlus size={17} />
              Фото в текст
            </button>
          </div>
          {link !== null && (
            <div className="editor-link">
              <input
                aria-label="Адрес ссылки"
                placeholder="https://"
                value={link}
                onChange={(e) => setLink(e.target.value)}
              />
              <button
                className="button"
                type="button"
                onClick={() => {
                  if (!/^(https?:\/\/|mailto:)/i.test(link)) {
                    notify(
                      "Укажите ссылку с https://, http:// или mailto:",
                      true,
                    );
                    return;
                  }
                  command("createLink", link);
                  setLink(null);
                }}
              >
                Вставить
              </button>
              <button
                type="button"
                aria-label="Отменить ссылку"
                onClick={() => setLink(null)}
              >
                <X size={16} />
              </button>
            </div>
          )}
          <div
            ref={editor}
            className="editor-body"
            role="textbox"
            aria-label="Сообщение"
            aria-multiline="true"
            contentEditable
            suppressContentEditableWarning
            tabIndex={0}
            data-placeholder="Здравствуйте!"
            onKeyUp={remember}
            onMouseUp={remember}
            onInput={remember}
            onPaste={(e) => {
              e.preventDefault();
              remember();
              const pasted = Array.from(e.clipboardData.files);
              if (pasted.length) {
                void attach(pasted, true);
                return;
              }
              const html = e.clipboardData.getData("text/html");
              command(
                "insertHTML",
                html
                  ? cleanHTML(html)
                  : plainHTML(e.clipboardData.getData("text/plain")),
              );
            }}
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => {
              e.preventDefault();
              remember();
              const dropped = Array.from(e.dataTransfer.files);
              if (dropped.length)
                void attach(
                  dropped,
                  dropped.every((f) => f.type.startsWith("image/")),
                );
            }}
          />
        </div>
        <div className="compose-attachments">
          {form.attachments?.map((a) => (
            <span key={a.id}>
              <Paperclip size={14} />
              {a.name}
              {a.inline && " · в тексте"}
              <button
                type="button"
                disabled={busy}
                aria-label={"Убрать " + a.name}
                onClick={() => remove(a)}
              >
                <X size={13} />
              </button>
            </span>
          ))}
        </div>
        <input
          ref={files}
          hidden
          type="file"
          multiple
          onChange={(e) => {
            void attach(Array.from(e.target.files || []));
            e.target.value = "";
          }}
        />
        <input
          ref={images}
          hidden
          type="file"
          accept="image/png,image/jpeg,image/gif,image/webp,image/bmp"
          multiple
          onChange={(e) => {
            void attach(Array.from(e.target.files || []), true);
            e.target.value = "";
          }}
        />
        {state.settings.demo && (
          <p className="info-strip">
            Деморежим: письмо останется внутри LeadHelper.
          </p>
        )}
        {smtpMissing && (
          <p className="info-strip">
            Чтение почты подключено. Отправка через SMTP ещё не настроена —
            письмо можно сохранить в черновики.
          </p>
        )}
        <div className="modal-footer">
          <button
            type="button"
            className="button"
            disabled={busy}
            onClick={() => files.current?.click()}
          >
            <Paperclip size={18} />
            Прикрепить файл
          </button>
          <span className="spacer" />
          <button
            type="button"
            className="button"
            disabled={busy}
            onClick={() => void save()}
          >
            Сохранить черновик
          </button>
          <button className="button primary" disabled={busy || smtpMissing}>
            <Send size={16} />
            {busy
              ? "Сохранение…"
              : state.settings.demo
                ? "Тестовая отправка"
                : "Отправить"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

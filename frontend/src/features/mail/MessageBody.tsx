import { useMemo, useState } from "react";
import type { Message } from "../../shared/types";
import { cleanHTML, resolveInline } from "./html";
import { splitQuotedHTML, splitQuotedText } from "./quotes";
export function MessageBody({ message }: { message: Message }) {
  const [remote, setRemote] = useState(false),
    [quoteOpen, setQuoteOpen] = useState(false);
  const parts = useMemo(
    () =>
      message.html
        ? splitQuotedHTML(message.html)
        : splitQuotedText(message.body),
    [message.html, message.body],
  );
  const render = (value: string) =>
    message.html ? (
      <div
        className="message-html"
        dangerouslySetInnerHTML={{
          __html: cleanHTML(
            value,
            (src) =>
              resolveInline(src, message.attachments || [], message.id) ||
              (remote && /^https?:\/\//i.test(src) ? src : null),
          ),
        }}
      />
    ) : (
      <div className="message-body">{value}</div>
    );
  const external =
    message.html &&
    /src\s*=\s*["']https?:/i.test(parts.body + (quoteOpen ? parts.quote : ""));
  return (
    <>
      {render(parts.body)}
      {parts.quote && (
        <div className="mail-quote">
          <button
            className="text-button quote-toggle"
            aria-expanded={quoteOpen}
            onClick={() => setQuoteOpen((v) => !v)}
          >
            {quoteOpen ? "Скрыть цитату" : "Показать цитату"}
          </button>
          {quoteOpen && render(parts.quote)}
        </div>
      )}
      {external && !remote && (
        <button
          className="text-button remote-images"
          onClick={() => setRemote(true)}
        >
          Показать внешние изображения (запрос к отправителю)
        </button>
      )}
    </>
  );
}

export function splitQuotedText(text: string): { body: string; quote: string } {
  const lines = text.split(/\r?\n/);
  let start = -1;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    if (
      /^>/.test(line) ||
      /^[-_─]{5,}\s*(Original Message|Исходное сообщение|Пересылаемое сообщение)?/i.test(
        line,
      ) ||
      /^On .{4,}wrote:$/i.test(line) ||
      /^.{4,}(написал[аи]?|пишет):\s*$/i.test(line) ||
      /^Begin forwarded message:/i.test(line)
    ) {
      start = i;
      break;
    }
    if (
      /^(From|От):\s*\S/i.test(line) &&
      lines
        .slice(i + 1, i + 7)
        .some((l) => /^(Sent|Date|To|Кому|Дата|Отправлено):/i.test(l.trim()))
    ) {
      start = i;
      break;
    }
  }
  return start < 0
    ? { body: text, quote: "" }
    : {
        body: lines.slice(0, start).join("\n").trimEnd(),
        quote: lines.slice(start).join("\n"),
      };
}
export function splitQuotedHTML(source: string): {
  body: string;
  quote: string;
} {
  const t = document.createElement("template");
  t.innerHTML = source;
  const quote = document.createElement("div");
  // Outlook puts the previous message after a reply header rather than inside a quote.
  const tail = t.content.querySelector(
    '[data-mail-quote="tail"],#divRplyFwdMsg,#replyForwardMsg',
  );
  if (tail) {
    const r = document.createRange();
    r.setStartBefore(tail);
    r.setEnd(t.content, t.content.childNodes.length);
    quote.appendChild(r.extractContents());
  }
  const blocks = [
    ...t.content.querySelectorAll(
      'blockquote,[data-mail-quote="block"],.gmail_quote,.yahoo_quoted,.protonmail_quote,[type="cite"]',
    ),
  ];
  for (const block of blocks)
    if (t.content.contains(block)) quote.appendChild(block);
  // Some clients quote as plain paragraphs in an HTML message. Detect a reply
  // header or quote markers only at a block boundary, and keep everything expandable.
  if (!quote.childNodes.length) {
    const marker = [...t.content.querySelectorAll("p,div")].find(
      (el) =>
        el.children.length === 0 && splitQuotedText(el.textContent || "").quote,
    );
    if (marker) {
      const r = document.createRange();
      r.setStartBefore(marker);
      r.setEnd(t.content, t.content.childNodes.length);
      quote.appendChild(r.extractContents());
    }
  }
  return { body: t.innerHTML, quote: quote.innerHTML };
}

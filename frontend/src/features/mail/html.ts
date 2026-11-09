import type { Attachment } from "../../shared/types";
import { attachmentURL } from "./AttachmentPreview";
const tags = new Set(
  "p div span br hr b strong i em u s strike blockquote pre code ul ol li a img table thead tbody tfoot tr td th h1 h2 h3 h4 h5 h6 sub sup".split(
    " ",
  ),
);
const blocked = new Set(
  "head script style template svg math iframe object embed form".split(" "),
);
export function escapeHTML(s: string) {
  return s.replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ]!,
  );
}
export function plainHTML(s: string) {
  return escapeHTML(s).replace(/\n/g, "<br>");
}
export function cleanHTML(
  source: string,
  resolveImage: (src: string) => string | null = () => null,
): string {
  const template = document.createElement("template");
  template.innerHTML = source;
  const output = document.createElement("div");
  const walk = (node: Node, parent: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      parent.appendChild(document.createTextNode(node.textContent || ""));
      return;
    }
    if (!(node instanceof Element)) return;
    const tag = node.tagName.toLowerCase();
    if (blocked.has(tag)) return;
    if (!tags.has(tag)) {
      node.childNodes.forEach((c) => walk(c, parent));
      return;
    }
    const el = document.createElement(tag);
    const quote = node.getAttribute("data-mail-quote");
    if (quote === "block" || quote === "tail")
      el.setAttribute("data-mail-quote", quote);
    if (tag === "a") {
      const href = node.getAttribute("href") || "";
      if (/^(https?:\/\/|mailto:)/i.test(href)) {
        el.setAttribute("href", href);
        el.setAttribute("target", "_blank");
        el.setAttribute("rel", "noopener noreferrer");
      }
    }
    if (tag === "img") {
      const src = resolveImage(node.getAttribute("src") || "");
      if (!src) {
        const label = document.createElement("span");
        label.className = "image-placeholder";
        label.textContent =
          node.getAttribute("alt") || "Изображение недоступно";
        parent.appendChild(label);
        return;
      }
      el.setAttribute("src", src);
      el.setAttribute(
        "alt",
        node.getAttribute("alt") || "Изображение в письме",
      );
      el.setAttribute("referrerpolicy", "no-referrer");
    }
    for (const key of ["colspan", "rowspan"]) {
      const value = node.getAttribute(key) || "";
      if ((tag === "td" || tag === "th") && /^\d{1,3}$/.test(value))
        el.setAttribute(key, value);
    }
    node.childNodes.forEach((c) => walk(c, el));
    parent.appendChild(el);
  };
  template.content.childNodes.forEach((n) => walk(n, output));
  return output.innerHTML;
}
export function resolveInline(
  src: string,
  files: Attachment[],
  messageId?: string,
) {
  if (!src.toLowerCase().startsWith("cid:")) return null;
  let cid = src.slice(4);
  try {
    cid = decodeURIComponent(cid);
  } catch {
    /* Keep malformed CID literal. */
  }
  const a = files.find(
    (a) => a.contentId?.replace(/^<|>$/g, "") === cid.replace(/^<|>$/g, ""),
  );
  if (!a || !/^image\/(png|jpeg|gif|webp|bmp)$/i.test(a.contentType))
    return null;
  if (a.data) return `data:${a.contentType};base64,${a.data}`;
  return messageId ? attachmentURL(messageId, a.id) + "/preview/content" : null;
}

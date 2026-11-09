import type { RemoteItem } from "../../shared/types";
export interface DepartmentNode {
  item: RemoteItem;
  children: DepartmentNode[];
}
export function departmentTree(items: RemoteItem[]): DepartmentNode[] {
  const nodes = new Map(
    items.map((item) => [item.id, { item, children: [] } as DepartmentNode]),
  );
  const parents = new Map<string, string>();
  for (const item of items)
    if (item.parent && item.parent !== item.id && nodes.has(item.parent))
      parents.set(item.id, item.parent);
  // A malformed or inaccessible parent must never hide a department or recurse forever.
  for (const id of nodes.keys()) {
    const seen = new Set<string>();
    let current: string | undefined = id;
    while (current) {
      if (seen.has(current)) {
        parents.delete(current);
        break;
      }
      seen.add(current);
      current = parents.get(current);
    }
  }
  const roots: DepartmentNode[] = [];
  for (const [id, node] of nodes) {
    const parent = parents.get(id);
    if (parent) nodes.get(parent)!.children.push(node);
    else roots.push(node);
  }
  return roots;
}
export function sortMessages(items: RemoteItem[]) {
  return [...items].sort((a, b) => {
    const diff =
      new Date(a.date || 0).getTime() - new Date(b.date || 0).getTime();
    return diff || a.id.localeCompare(b.id, undefined, { numeric: true });
  });
}
export function plainRemoteText(text = "") {
  return remoteNodesText(parseRemoteText(text))
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

export type RemoteTextNode =
  | string
  | { tag: string; value: string; children: RemoteTextNode[] };
const remoteTags = new Set(
  "B I U S P QUOTE CODE LIST IMG URL SIZE COLOR FONT CENTER LEFT RIGHT JUSTIFY TABLE TR TD TH USER CHAT CONTEXT SEND PUT CALL BR ICON TIMESTAMP DISK FILE ATTACH ENTITY MENTION COMMAND *".split(
    " ",
  ),
);
const blockTags = new Set(
  "P QUOTE LIST CENTER LEFT RIGHT JUSTIFY TABLE TR".split(" "),
);
function entities(value: string) {
  const named: Record<string, string> = {
    amp: "&",
    lt: "<",
    gt: ">",
    quot: '"',
    apos: "'",
    nbsp: " ",
  };
  return value.replace(
    /&(#x[0-9a-f]+|#\d+|amp|lt|gt|quot|apos|nbsp);/gi,
    (whole, code: string) => {
      if (code[0] !== "#") return named[code.toLowerCase()] || whole;
      const n =
        code[1].toLowerCase() === "x"
          ? parseInt(code.slice(2), 16)
          : Number(code.slice(1));
      return n > 0 && n <= 0x10ffff && !(n >= 0xd800 && n <= 0xdfff)
        ? String.fromCodePoint(n)
        : "";
    },
  );
}
export function safeTextURL(value: string) {
  try {
    const u = new URL(value);
    if (
      !/^https?:$/.test(u.protocol) ||
      u.username ||
      u.password ||
      /\/rest\//i.test(u.pathname)
    )
      return "";
    for (const key of u.searchParams.keys())
      if (/^(auth|auth_id|access_token|refresh_token)$/i.test(key)) return "";
    return u.href;
  } catch {
    return "";
  }
}
function attribute(value: string, name?: string) {
  const expression = name
    ? new RegExp(
        `(?:^|\\s)${name}\\s*=\\s*(?:"([^"]*)"|'([^']*)'|([^\\s]+))`,
        "i",
      )
    : /^\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s]+))/;
  const match = value.match(expression);
  return match ? (match[1] ?? match[2] ?? match[3] ?? "") : "";
}
export function remoteNodesText(nodes: RemoteTextNode[]): string {
  return nodes
    .map((n) =>
      typeof n === "string"
        ? n
        : n.tag === "IMG"
          ? "Изображение"
          : blockTags.has(n.tag)
            ? "\n" + remoteNodesText(n.children) + "\n"
            : remoteNodesText(n.children),
    )
    .join("");
}
// Provider markup is parsed into a bounded tree of text, never injected as HTML.
// Unknown square-bracket text and literal code samples remain intact.
export function parseRemoteText(source = ""): RemoteTextNode[] {
  const root: RemoteTextNode[] = [];
  const stack: { tag: string; children: RemoteTextNode[] }[] = [
    { tag: "", children: root },
  ];
  const add = (n: RemoteTextNode) => stack[stack.length - 1].children.push(n);
  const htmlTags: Record<string, string> = {
    strong: "B",
    b: "B",
    em: "I",
    i: "I",
    u: "U",
    s: "S",
    p: "P",
    div: "P",
    blockquote: "QUOTE",
    ul: "LIST",
    ol: "LIST",
  };
  // Legacy feed/card HTML is reduced to the same formatting vocabulary.
  // CODE is protected before any HTML or BBCode handling.
  const codeBlocks: string[] = [];
  let text = source.replace(
    /\[CODE(?:[ =][^\]\r\n]*)?\]([\s\S]*?)(?:\[\/CODE\]|$)/gi,
    (_, body: string) => {
      codeBlocks.push(body);
      return `\u0000${codeBlocks.length - 1}\u0000`;
    },
  );
  text = text
    .replace(/<(script|style|iframe|object)\b[^>]*>[\s\S]*?<\/\1\s*>/gi, "")
    .replace(/<!--([\s\S]*?)-->/g, "")
    .replace(
      /<(\/?)([a-z][\w-]*)\b([^>]*)>/gi,
      (whole, close: string, raw: string, attrs: string) => {
        const name = raw.toLowerCase();
        if (name === "br") return "[BR]";
        if (name === "li") return close ? "" : "[*]";
        if (name === "a")
          return close
            ? "[/URL]"
            : `[URL="${attribute(attrs, "href").replace(/[\[\]"]/g, "")}"]`;
        if (name === "img") return "Изображение";
        if (htmlTags[name]) return `[${close}${htmlTags[name]}]`;
        return [
          "span",
          "table",
          "tbody",
          "thead",
          "tr",
          "td",
          "th",
          "hr",
          "script",
          "style",
          "iframe",
          "object",
        ].includes(name)
          ? name === "tr" && close
            ? "[BR]"
            : ""
          : whole;
      },
    );
  const tokens =
    /\x00(\d+)\x00|\[(\/?)([a-z][a-z0-9_-]*|\*)(?=[\s=\]])([^\]\r\n]*)\]/gi;
  let position = 0;
  for (const match of text.matchAll(tokens)) {
    add(entities(text.slice(position, match.index)));
    position = match.index! + match[0].length;
    if (match[1] !== undefined) {
      add({
        tag: "CODE",
        value: "",
        children: [codeBlocks[Number(match[1])] || ""],
      });
      continue;
    }
    const tag = match[3].toUpperCase(),
      attrs = match[4];
    if (!remoteTags.has(tag)) {
      add(match[0]);
      continue;
    }
    if (match[2]) {
      const index = stack.map((n) => n.tag).lastIndexOf(tag);
      if (index > 0) stack.length = index;
      continue;
    }
    if (tag === "BR" || tag === "*") {
      add(tag === "BR" ? "\n" : "\n• ");
      continue;
    }
    if (tag === "DISK" || tag === "FILE" || tag === "ATTACH") {
      add("📎 Вложение");
      continue;
    }
    if (tag === "ICON") {
      add(entities(attribute(attrs, "title")));
      continue;
    }
    if (tag === "TIMESTAMP") {
      const value = Number(attribute(attrs)),
        d = new Date(value * 1000);
      if (Number.isFinite(value) && value > 0 && Number.isFinite(d.getTime()))
        add(d.toLocaleString("ru-RU", { timeZone: "Europe/Moscow" }));
      continue;
    }
    const n = {
      tag,
      value: entities(attribute(attrs)),
      children: [] as RemoteTextNode[],
    };
    add(n);
    if (stack.length < 32) stack.push(n);
  }
  // Older cached notifications may have been truncated in the middle of a tag.
  add(
    entities(
      text
        .slice(position)
        .replace(
          /\[\/?(?:USER|CHAT|CONTEXT|URL|DISK|ICON|TIMESTAMP)(?:[=\s][^\]]*)?$/gi,
          "",
        ),
    ),
  );
  return root;
}

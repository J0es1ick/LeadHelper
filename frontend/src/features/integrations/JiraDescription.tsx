import { useEffect, useRef } from "react";

type ADFNode = {
  type: string;
  text?: string;
  attrs?: Record<string, unknown>;
  marks?: { type: string; attrs?: Record<string, unknown> }[];
  content?: ADFNode[];
};
function inline(
  nodes: Node[],
  marks: NonNullable<ADFNode["marks"]> = [],
): ADFNode[] {
  return nodes.flatMap((node) => {
    if (node.nodeType === Node.TEXT_NODE)
      return node.textContent
        ? [
            {
              type: "text",
              text: node.textContent,
              ...(marks.length ? { marks } : {}),
            },
          ]
        : [];
    if (!(node instanceof HTMLElement)) return [];
    if (node.tagName === "BR") return [{ type: "hardBreak" }];
    const tag = node.tagName,
      next = [...marks];
    if (["B", "STRONG"].includes(tag)) next.push({ type: "strong" });
    if (["I", "EM"].includes(tag)) next.push({ type: "em" });
    if (["S", "STRIKE", "DEL"].includes(tag)) next.push({ type: "strike" });
    if (tag === "CODE") next.push({ type: "code" });
    if (tag === "A" && /^https?:\/\//i.test(node.getAttribute("href") || ""))
      next.push({ type: "link", attrs: { href: node.getAttribute("href")! } });
    return inline(Array.from(node.childNodes), next);
  });
}
function blocks(nodes: Node[]): ADFNode[] {
  const out: ADFNode[] = [];
  let run: Node[] = [];
  const flush = () => {
    if (run.length) {
      const content = inline(run);
      out.push({ type: "paragraph", ...(content.length ? { content } : {}) });
      run = [];
    }
  };
  for (const node of nodes) {
    if (
      node instanceof HTMLElement &&
      ["UL", "OL", "DIV", "P", "BLOCKQUOTE"].includes(node.tagName)
    ) {
      flush();
      if (["UL", "OL"].includes(node.tagName))
        out.push({
          type: node.tagName === "UL" ? "bulletList" : "orderedList",
          content: Array.from(node.children)
            .filter((n) => n.tagName === "LI")
            .map((n) => ({
              type: "listItem",
              content: blocks(Array.from(n.childNodes)),
            })),
        });
      else if (node.tagName === "BLOCKQUOTE")
        out.push({
          type: "blockquote",
          content: blocks(Array.from(node.childNodes)),
        });
      else out.push(...blocks(Array.from(node.childNodes)));
    } else run.push(node);
  }
  flush();
  return out.length ? out : [{ type: "paragraph" }];
}
function wiki(node: ADFNode, list = ""): string {
  if (node.type === "text") {
    let text = node.text || "";
    for (const mark of node.marks || []) {
      const marker = { strong: "*", em: "_", strike: "-", code: "{{" }[
        mark.type
      ];
      if (mark.type === "link") text = `[${text}|${mark.attrs?.href}]`;
      else if (marker)
        text = marker + text + (mark.type === "code" ? "}}" : marker);
    }
    return text;
  }
  if (node.type === "hardBreak") return "\\\\\n";
  if (["bulletList", "orderedList"].includes(node.type))
    return (node.content || [])
      .map((n) => wiki(n, list + (node.type === "bulletList" ? "*" : "#")))
      .join("");
  if (node.type === "listItem")
    return list + " " + (node.content || []).map((n) => wiki(n, list)).join("");
  const body = (node.content || []).map((n) => wiki(n, list)).join("");
  return node.type === "paragraph"
    ? body + "\n"
    : node.type === "blockquote"
      ? "{quote}" + body + "{quote}\n"
      : body;
}
function textOf(v: unknown): string {
  if (typeof v === "string") return v;
  if (!v || typeof v !== "object") return "";
  const n = v as ADFNode;
  return (
    (n.text || "") +
    (n.content || []).map(textOf).join(n.type === "doc" ? "\n" : "")
  );
}
export function JiraDescription({
  id,
  name,
  value,
  cloud,
  onChange,
  disabled,
}: {
  id: string;
  name: string;
  value: unknown;
  cloud: boolean;
  onChange: (v: unknown) => void;
  disabled: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null),
    initial = useRef(value);
  useEffect(() => {
    if (ref.current) ref.current.innerText = textOf(initial.current);
  }, []);
  function commit() {
    if (!ref.current) return;
    const doc = {
      type: "doc",
      version: 1,
      content: blocks(Array.from(ref.current.childNodes)),
    };
    onChange(cloud ? doc : wiki(doc).trimEnd());
  }
  return (
    <div className="jira-rich-editor">
      <div
        className="jira-rich-toolbar"
        role="toolbar"
        aria-label="Форматирование описания"
      >
        {[
          ["bold", "Жирный", "B"],
          ["italic", "Курсив", "I"],
          ["strikeThrough", "Зачёркнутый", "S"],
          ["insertUnorderedList", "Маркированный список", "• Список"],
          ["insertOrderedList", "Нумерованный список", "1. Список"],
          ["removeFormat", "Убрать форматирование", "Очистить"],
        ].map(([command, title, text]) => (
          <button
            key={command}
            type="button"
            aria-label={title}
            title={title}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => {
              ref.current?.focus();
              document.execCommand(command);
              commit();
            }}
          >
            {text}
          </button>
        ))}
      </div>
      <div
        ref={ref}
        id={id}
        role="textbox"
        aria-label={name}
        aria-multiline="true"
        contentEditable={!disabled}
        suppressContentEditableWarning
        className="jira-description"
        onInput={commit}
        onPaste={(e) => {
          e.preventDefault();
          document.execCommand(
            "insertText",
            false,
            e.clipboardData.getData("text/plain"),
          );
          commit();
        }}
      />
      <small className="muted">
        Форматирование сохраняется в{" "}
        {cloud ? "документе Jira Cloud" : "wiki-разметке Jira"}.
      </small>
    </div>
  );
}

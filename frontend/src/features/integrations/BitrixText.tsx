import { Fragment, type ReactNode } from "react";
import {
  parseRemoteText,
  remoteNodesText,
  safeTextURL,
  type RemoteTextNode,
} from "./bitrix-model";

export function BitrixText({
  text = "",
  className = "",
}: {
  text?: string;
  className?: string;
}) {
  function render(nodes: RemoteTextNode[], linked = false): ReactNode {
    return nodes.map((n, i) => {
      if (typeof n === "string")
        return (
          <Fragment key={i}>
            {linked
              ? n
              : n.split(/(https?:\/\/[^\s<>\[\]]+)/gi).map((part, j) => {
                  const candidate = part.replace(/[.,;!?)]+$/, ""),
                    url = safeTextURL(candidate);
                  return url ? (
                    <Fragment key={j}>
                      <a href={url} target="_blank" rel="noreferrer">
                        {candidate}
                      </a>
                      {part.slice(candidate.length)}
                    </Fragment>
                  ) : (
                    part
                  );
                })}
          </Fragment>
        );
      const children = render(n.children, linked || n.tag === "URL");
      switch (n.tag) {
        case "B":
          return <b key={i}>{children}</b>;
        case "I":
          return <em key={i}>{children}</em>;
        case "U":
          return <u key={i}>{children}</u>;
        case "S":
          return <s key={i}>{children}</s>;
        case "CODE":
          return (
            <code className="bx-code" key={i}>
              {remoteNodesText(n.children)}
            </code>
          );
        case "QUOTE":
          return <blockquote key={i}>{children}</blockquote>;
        case "P":
        case "LIST":
        case "CENTER":
        case "LEFT":
        case "RIGHT":
        case "JUSTIFY":
        case "TR":
        case "TABLE":
          return <div key={i}>{children}</div>;
        case "USER":
        case "MENTION":
          return (
            <span className="bx-mention" key={i}>
              {children}
            </span>
          );
        case "IMG": {
          const url = safeTextURL(remoteNodesText(n.children).trim());
          return url ? (
            <a key={i} href={url} target="_blank" rel="noreferrer">
              <img
                className="bx-inline-image"
                src={url}
                alt="Изображение"
                loading="lazy"
                referrerPolicy="no-referrer"
              />
            </a>
          ) : (
            <span key={i}>Изображение</span>
          );
        }
        case "URL": {
          const url = safeTextURL(
            n.value || remoteNodesText(n.children).trim(),
          );
          return url && !linked ? (
            <a key={i} href={url} target="_blank" rel="noreferrer">
              {remoteNodesText(n.children) ? children : url}
            </a>
          ) : (
            <Fragment key={i}>{children}</Fragment>
          );
        }
        default:
          return <Fragment key={i}>{children}</Fragment>;
      }
    });
  }
  return (
    <div className={"bx-formatted-text " + className}>
      {render(parseRemoteText(text))}
    </div>
  );
}

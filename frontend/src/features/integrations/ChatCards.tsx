import { useEffect, useState } from "react";
import type { RemoteItem } from "../../shared/types";
import { Avatar, Modal } from "../../shared/ui";
import { BitrixText } from "./BitrixText";
import { endpoint } from "../../shared/api";
import { bitrixPage } from "./bitrix-cache";
import { poll, unchanged } from "../../shared/live";
import { plainRemoteText } from "./bitrix-model";
import { ChatAttachments } from "./ChatAttachments";

export function ChatCards({
  item,
  references = true,
}: {
  item: RemoteItem;
  references?: boolean;
}) {
  const [selected, setSelected] = useState<
    NonNullable<RemoteItem["references"]>[number] | null
  >(null);
  return (
    <div className="bx-rich-cards">
      {item.cards?.map((card, i) => (
        <details className="bx-rich-card" key={i}>
          <summary>
            {plainRemoteText(card.title) || "Вложение"} <span>Раскрыть</span>
          </summary>
          <div className="bx-rich-content">
            {card.blocks.map((block, j) =>
              block.kind === "delimiter" ? (
                <hr key={j} />
              ) : (
                <div className={"bx-rich-block " + block.kind} key={j}>
                  {block.title && (
                    <strong>{plainRemoteText(block.title)}</strong>
                  )}
                  {block.text && <BitrixText text={block.text} />}
                  {block.url && (
                    <a href={block.url} target="_blank" rel="noreferrer">
                      {block.kind === "file"
                        ? "Открыть файл"
                        : block.kind === "image"
                          ? "Открыть изображение"
                          : "Открыть ссылку"}{" "}
                      ↗
                    </a>
                  )}
                </div>
              ),
            )}
          </div>
        </details>
      ))}
      {references &&
        item.references?.map((ref) => (
          <button
            className="button small bx-reference"
            key={ref.dialogId + ":" + ref.messageId}
            onClick={() => setSelected(ref)}
          >
            Развернуть связанное сообщение
          </button>
        ))}
      {selected && (
        <MessagePreview reference={selected} close={() => setSelected(null)} />
      )}
    </div>
  );
}
function MessagePreview({
  reference,
  close,
}: {
  reference: NonNullable<RemoteItem["references"]>[number];
  close: () => void;
}) {
  const [item, setItem] = useState<RemoteItem | null>(null),
    [loading, setLoading] = useState(true),
    [error, setError] = useState("");
  useEffect(() => {
    const abort = new AbortController();
    poll(
      () =>
        bitrixPage(
          `/workspace/bitrix?kind=message&id=${endpoint(reference.dialogId)}&cursor=${endpoint(reference.messageId)}`,
          abort.signal,
        )
          .then((page) => {
            if (!abort.signal.aborted) {
              setItem((old) =>
                unchanged(
                  old,
                  page.items.find((m) => m.id === reference.messageId) || null,
                ),
              );
              setError("");
            }
          })
          .catch((e) => {
            if (!abort.signal.aborted) setError(e.message);
            throw e;
          })
          .finally(() => {
            if (!abort.signal.aborted) setLoading(false);
          }),
      { signal: abort.signal, interval: 15000 },
    );
    return () => abort.abort();
  }, [reference.dialogId, reference.messageId]);
  return (
    <Modal title="Вложение Битрикс24" onClose={close} wide>
      <div className="bx-message-preview">
        {loading ? (
          <p role="status">Загружаем сообщение…</p>
        ) : item ? (
          <>
            {!item.system && (
              <header className="bx-message-author">
                <Avatar name={item.title} src={item.avatarUrl} size="small" />
                <strong>{item.title}</strong>
              </header>
            )}
            <BitrixText text={item.text} />
            <ChatCards item={item} references={false} />
            <ChatAttachments files={item.attachments} />
            {!item.cards?.length && !item.attachments?.length && (
              <p className="muted">
                Дополнительные материалы звонка можно открыть в Битриксе.
              </p>
            )}
          </>
        ) : (
          <p role="status">
            {error ||
              "Сообщение пока недоступно. Попробуйте позже или откройте оригинал."}
          </p>
        )}
        <a
          className="button"
          href={reference.url}
          target="_blank"
          rel="noreferrer"
        >
          Открыть оригинал в Битрикс24 ↗
        </a>
      </div>
    </Modal>
  );
}

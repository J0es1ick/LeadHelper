import { useEffect, useRef, useState, type ReactNode } from "react";
import { X, ArrowUpRight, Inbox } from "lucide-react";
import type { Member, Task } from "./types";
import { priorities, sources } from "./types";
import { dayLabel, initials, dateKey } from "./api";
export function Avatar({
  member,
  name,
  src,
  size = "",
}: {
  member?: Member;
  name?: string;
  src?: string;
  size?: string;
}) {
  const [failed, setFailed] = useState("");
  const photo = src && src !== failed && /^https:\/\//i.test(src) ? src : "";
  return (
    <span
      className={"avatar " + size}
      style={{ background: member?.color || "#708b7d" }}
      title={member?.name || name}
    >
      {photo ? (
        <img
          key={photo}
          src={photo}
          alt=""
          loading="lazy"
          decoding="async"
          referrerPolicy="no-referrer"
          onError={() => setFailed(photo)}
        />
      ) : (
        initials(member?.name || name || "?")
      )}
    </span>
  );
}
export function Source({ value }: { value: string }) {
  return (
    <span className={"source source-" + value}>
      <i />
      {sources[value] || value}
    </span>
  );
}
export function PriorityBadge({ value }: { value: Task["priority"] }) {
  return (
    <span className={"priority priority-" + value}>
      <span>▰</span>
      {priorities[value]}
    </span>
  );
}
export function Empty({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <Inbox size={26} />
      </div>
      <h3>{title}</h3>
      {description && <p>{description}</p>}
      {action}
    </div>
  );
}
export function SectionHead({
  title,
  subtitle,
  action,
}: {
  title: string;
  subtitle?: string;
  action?: ReactNode;
}) {
  return (
    <div className="section-heading">
      <div>
        <h2>{title}</h2>
        {subtitle && <p>{subtitle}</p>}
      </div>
      {action}
    </div>
  );
}
export function Modal({
  title,
  children,
  onClose,
  wide = false,
  className = "",
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;
  useEffect(() => {
    const previous = document.activeElement as HTMLElement;
    const root = ref.current;
    const focusables = () =>
      Array.from(
        root?.querySelectorAll<HTMLElement>(
          'button,input,textarea,select,a[href],[tabindex="0"]',
        ) || [],
      ).filter((x) => !x.hasAttribute("disabled"));
    focusables()[0]?.focus();
    const key = (e: KeyboardEvent) => {
      if (
        Array.from(document.querySelectorAll('[role="dialog"]')).at(-1) !== root
      )
        return;
      if (e.key === "Escape") {
        closeRef.current();
      }
      if (e.key === "Tab") {
        const all = focusables();
        if (!all.length) return;
        if (e.shiftKey && document.activeElement === all[0]) {
          e.preventDefault();
          all.at(-1)?.focus();
        } else if (!e.shiftKey && document.activeElement === all.at(-1)) {
          e.preventDefault();
          all[0]?.focus();
        }
      }
    };
    document.addEventListener("keydown", key);
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", key);
      document.body.style.overflow = overflow;
      previous?.focus();
    };
  }, []);
  return (
    <div
      className="modal-backdrop"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        className={"modal " + (wide ? "wide " : "") + className}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        ref={ref}
      >
        <div className="modal-header">
          <h2>{title}</h2>
          <button
            className="icon-button"
            onClick={onClose}
            aria-label="Закрыть"
          >
            <X size={20} />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}
export function TaskMini({
  task,
  members,
  onClick,
  onToggle,
}: {
  task: Task;
  members: Member[];
  onClick: () => void;
  onToggle?: () => void;
}) {
  return (
    <div className="task-mini">
      {onToggle ? (
        <button
          className={"check " + (task.status === "done" ? "checked" : "")}
          onClick={onToggle}
          aria-label={"Завершить: " + task.title}
        >
          {task.status === "done" ? "✓" : ""}
        </button>
      ) : (
        <span className={"status-dot " + task.status} />
      )}
      <button
        className={
          "task-mini-title " + (task.status === "done" ? "completed" : "")
        }
        onClick={onClick}
      >
        {task.title}
        <span>
          {task.project} {task.externalId ? " · " + task.externalId : ""}
        </span>
      </button>
      <span
        className={
          task.dueDate && task.dueDate < dateKey() && task.status !== "done"
            ? "overdue date-label"
            : "date-label"
        }
      >
        {dayLabel(task.dueDate)}
      </span>
      {task.assigneeId && (
        <Avatar
          member={members.find((m) => m.id === task.assigneeId)}
          size="small"
        />
      )}
    </div>
  );
}
export function ViewLink({
  children,
  onClick,
}: {
  children: ReactNode;
  onClick: () => void;
}) {
  return (
    <button className="text-button" onClick={onClick}>
      {children}
      <ArrowUpRight size={15} />
    </button>
  );
}

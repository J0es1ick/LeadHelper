export type Page =
  | "dashboard"
  | "planner"
  | "backlog"
  | "projects"
  | "team"
  | "mail"
  | "notes"
  | "reports"
  | "integrations"
  | "bitrix"
  | "confluence"
  | "settings";
export type Status = "backlog" | "todo" | "progress" | "review" | "done";
export type Priority = "low" | "medium" | "high" | "critical";
export interface Task {
  id: string;
  title: string;
  description: string;
  status: Status;
  priority: Priority;
  source: string;
  externalId?: string;
  url?: string;
  project: string;
  assigneeId: string;
  estimate: number;
  dueDate: string;
  plannedDate: string;
  personal: boolean;
  tags: string[];
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
  demo: boolean;
}
export interface TeamDepartment {
  id: string;
  name: string;
  parent?: string;
}
export interface TeamSyncStatus {
  enabled: boolean;
  accountId?: string;
  managerName?: string;
  roots?: TeamDepartment[];
  departmentCount: number;
  memberCount: number;
  lastAttemptAt?: string;
  lastSuccessAt?: string;
  error?: string;
}
export interface Member {
  email?: string;
  jiraUserId?: string;
  jiraLogin?: string;
  jiraName?: string;
  jiraSite?: string;
  source?: string;
  externalId?: string;
  accountId?: string;
  inactive?: boolean;
  departments?: TeamDepartment[];
  id: string;
  name: string;
  role: string;
  color: string;
  capacity: number;
  demo: boolean;
}
export interface Note {
  id: string;
  title: string;
  body: string;
  tag: string;
  pinned: boolean;
  updatedAt: string;
  demo: boolean;
}
export interface Attachment {
  id: string;
  name: string;
  contentType: string;
  data: string;
  blobKey?: string;
  size?: number;
  contentId?: string;
  inline?: boolean;
}
export interface Message {
  id: string;
  externalId?: string;
  from: string;
  fromName: string;
  to: string;
  cc: string;
  subject: string;
  body: string;
  folder: string;
  html?: string;
  messageId?: string;
  inReplyTo?: string;
  references?: string;
  accountId?: string;
  read: boolean;
  starred: boolean;
  date: string;
  attachments: Attachment[];
  delivery?: string;
  demo: boolean;
  sourceFolder?: string;
  importWarning?: string;
}
export interface MailSyncStatus {
  state: "inactive" | "syncing" | "idle" | "error" | "";
  startedAt?: string;
  lastSuccessAt?: string;
  updatedAt?: string;
  folder?: string;
  imported: number;
  error?: string;
}
export interface Settings {
  name: string;
  workspace: string;
  email: string;
  demo: boolean;
  modules: Record<string, boolean>;
}
export interface SyncLog {
  id: string;
  provider: string;
  at: string;
  success: boolean;
  count: number;
  message: string;
  demo: boolean;
}
export interface State {
  revision?: string;
  taskMetrics?: TaskMetrics;
  teamSync?: TeamSyncStatus;
  version: number;
  settings: Settings;
  tasks: Task[];
  members: Member[];
  notes: Note[];
  messages: Message[];
  syncLogs: SyncLog[];
  mailSync: MailSyncStatus;
  mailConnection?: Connection;
  notifications?: InboxNotification[];
  mailStats?: {
    folders: Record<string, number>;
    unread: number;
    revision: number;
  };
}
export interface Connection {
  id: string;
  name: string;
  configured: boolean;
  hint: string;
  email?: string;
  host?: string;
  sendConfigured?: boolean;
}
export interface InboxNotification {
  id: string;
  provider: string;
  title: string;
  text: string;
  target: string;
  at: string;
  read: boolean;
}
export interface RemoteItem {
  avatarUrl?: string;
  headAvatarUrl?: string;
  system?: boolean;
  cards?: {
    title: string;
    blocks: { kind: string; title?: string; text?: string; url?: string }[];
  }[];
  references?: { dialogId: string; messageId: string; url: string }[];
  readMessageId?: string;
  attachments?: RemoteAttachment[];
  own?: boolean;
  status?: string;
  online?: boolean;
  departments?: string[];
  headId?: string;
  headName?: string;
  headUrl?: string;
  sort?: number;
  id: string;
  title: string;
  text?: string;
  html?: string;
  url?: string;
  kind?: string;
  parent?: string;
  author?: string;
  date?: string;
  unread?: number;
  messageId?: string;
}
export interface RemoteAttachment {
  id: string;
  name: string;
  size: number;
  image: boolean;
  url: string;
}
export interface JiraUser {
  id: string;
  login: string;
  name: string;
  email?: string;
}
export interface RemotePage {
  accountId?: string;
  total?: number;
  totalAll?: number;
  warning?: string;
  items: RemoteItem[];
  next?: string;
}
export interface CalendarEvent {
  id: string;
  title: string;
  start: string;
  end: string;
  allDay: boolean;
  busy: boolean;
  tentative: boolean;
  location?: string;
  description?: string;
  url: string;
}
export interface CalendarPage {
  events: CalendarEvent[];
  updatedAt: string;
  timezone: string;
}
export interface MailThread {
  id: string;
  latest: Message;
  count: number;
  unread: number;
  starred: boolean;
  hasAttachments: boolean;
}
export interface MailPage {
  items: MailThread[];
  nextCursor?: string;
  limit: number;
}
export interface MessagePage {
  items: Message[];
  nextCursor?: string;
  thread: MailThread;
}
export interface Report {
  from: string;
  to: string;
  total: number;
  completed: number;
  inProgress: number;
  overdue: number;
  plannedHours: number;
  workload: { member: Member; hours: number; tasks: number; percent: number }[];
  tasks: Task[];
}
export const statuses: Record<Status, string> = {
  backlog: "Бэклог",
  todo: "К выполнению",
  progress: "В работе",
  review: "На проверке",
  done: "Готово",
};
export const priorities: Record<Priority, string> = {
  low: "Низкий",
  medium: "Средний",
  high: "Высокий",
  critical: "Критический",
};
export const sources: Record<string, string> = {
  local: "LeadHelper",
  jira: "Jira",
  bitrix: "Битрикс24",
};
export interface TaskPage {
  items: Task[];
  total: number;
  page: number;
  limit: number;
  pages: number;
}
export interface TaskMetrics {
  worklogAvailable?: boolean;
  worklogHours?: Record<string, number>;
  worklogSyncedAt?: string;
  statuses: Record<string, number>;
  overdue: number;
  personalTotal: number;
  personalDone: number;
  memberTotals: Record<string, number>;
  memberActive: Record<string, number>;
  memberHours: Record<string, number>;
}
export interface TeamPlanEntry {
  memberId: string;
  date: string;
  taskId: string;
  hours: number;
}
export interface PlannedTask extends TeamPlanEntry {
  task: Task;
}
export interface TeamPlanCell {
  memberId: string;
  date: string;
  count: number;
  hours: number;
  unestimated: number;
  tasks: Task[];
}
export interface TeamWeekPlan {
  cells: TeamPlanCell[];
}
export interface TeamDayPlan {
  candidates: TaskPage;
  selected: PlannedTask[];
  revision: string;
}

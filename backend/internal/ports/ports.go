// Package ports defines dependencies required by application use cases.
package ports

import (
	"context"
	"io"
	"leadhelper/internal/domain"
	"time"
)

type JiraIssueSource interface {
	SiteID() string
	CreationProjects(context.Context) ([]domain.JiraOption, error)
	CreationTypes(context.Context, string) ([]domain.JiraOption, error)
	CreationFields(context.Context, string, string) (domain.JiraCreateMeta, error)
	AssignableUsers(context.Context, string, string) ([]domain.JiraUser, error)
	CreateIssue(context.Context, domain.JiraCreateInput) (domain.JiraCreateResult, error)
	UploadIssueFiles(context.Context, string, []domain.JiraUpload) []string
}
type JiraWorklogSource interface {
	SiteID() string
	FetchWorklogs(context.Context, []domain.Member, string, string) ([]domain.Worklog, error)
}
type JiraFeatureRepository interface {
	JiraWrite(context.Context, string, string) (domain.JiraWriteRecord, error)
	PutJiraWrite(context.Context, string, string, domain.JiraWriteRecord) error
	ReplaceWorklogs(context.Context, string, string, string, []domain.Worklog, time.Time) error
	WorklogWeek(context.Context, string, string, string) (domain.WorklogWeek, error)
	WorklogDay(context.Context, string, string, string, int, int) (domain.WorklogPage, error)
}
type WorkspaceFileSource interface {
	DownloadFile(context.Context, string) (io.ReadCloser, string, string, int64, error)
}
type WorkspaceReadSource interface {
	ReadChat(context.Context, string, string) (domain.ChatRead, error)
}
type ProjectRepository interface {
	Projects(context.Context, bool, domain.ProjectQuery) (domain.ProjectPage, error)
	ProjectByName(context.Context, bool, string) (domain.Project, error)
	ProjectByNameInGroup(context.Context, bool, string, string) (domain.Project, error)
	SaveProjects(context.Context, bool, []domain.Project, string) error
}
type ProjectSyncRepository interface {
	ProjectDetail(context.Context, bool, string) (domain.Project, error)
	MergeProjectSheet(context.Context, domain.ProjectSheetSnapshot, []domain.Project, bool) error
	PendingProjectChanges(context.Context, string) ([]domain.ProjectSheetChange, error)
	ProjectSyncStatus(context.Context) (domain.ProjectSyncStatus, error)
	ProjectSyncError(context.Context, string) error
	ResolveProjectConflict(context.Context, string, int, bool) error
}
type ProjectSheetSource interface {
	Configured() bool
	Snapshot(context.Context) (domain.ProjectSheetSnapshot, error)
	Apply(context.Context, domain.ProjectSheetChange) error
}

// Repository serializes write transactions and rolls back failed callbacks.
type Repository interface {
	View(context.Context) (domain.State, error)
	Update(context.Context, func(*domain.State) error) error
}
type TaskSource interface {
	Configured() bool
	FetchTasks(context.Context) ([]domain.Task, []domain.Member, error)
}

// Read models avoid transferring or decoding the full task archive for each UI page.
type PagedTaskRepository interface {
	ViewShell(context.Context) (domain.State, error)
	TaskPage(context.Context, domain.TaskQuery) (domain.TaskPage, error)
	Task(context.Context, string, bool) (domain.Task, error)
	TaskMetrics(context.Context, domain.TaskQuery, string) (domain.TaskMetrics, error)
	PlanEntries(context.Context, bool, string, string, string) ([]domain.TeamPlanEntry, error)
}
type TeamTaskSource interface {
	TaskSource
	TeamScoped() bool
	SiteID() string
	SearchUsers(context.Context, string) ([]domain.JiraUser, error)
	User(context.Context, string) (domain.JiraUser, error)
	FetchTeamTasks(context.Context, []domain.Member) ([]domain.Task, error)
}
type WorkspaceSource interface {
	Configured() bool
	AccountID() string
	Browse(context.Context, domain.RemoteQuery) (domain.RemotePage, error)
	Calendar(context.Context, string, string) (domain.CalendarPage, error)
	SendChat(context.Context, string, string) error
}

type WorkspaceFileSender interface {
	SendChatFile(context.Context, string, domain.ChatUpload) (domain.ChatUploadResult, error)
}
type TeamSource interface {
	FetchTeam(context.Context) (domain.TeamSnapshot, error)
}
type KnowledgeSource interface {
	Configured() bool
	Browse(context.Context, domain.RemoteQuery) (domain.RemotePage, error)
}
type Configurator interface {
	Configure(context.Context, string, domain.IntegrationConfig) error
	PublicConfig(string) domain.IntegrationConfig
}
type MailProvider interface {
	Configured() bool
	// ValidateSend checks local configuration and limits without external I/O.
	ValidateSend(domain.Message) error
	FetchMessages(context.Context) ([]domain.Message, error)
	Send(context.Context, domain.Message) error
}

// IncrementalMailProvider commits bounded pages; the caller persists progress.
type IncrementalMailProvider interface {
	MailProvider
	AccountID() string
	Sync(context.Context, map[string]domain.MailCheckpoint, func(domain.MailBatch) error) error
}
type BlobStore interface {
	Put(context.Context, []byte) (string, error)
	Get(context.Context, string) ([]byte, error)
}

// PagedMailRepository keeps large mailboxes out of general state snapshots.
type PagedMailRepository interface {
	Repository
	ViewCore(context.Context) (domain.State, error)
	UpdateCore(context.Context, func(*domain.State) error) error
	UpdateMessages(context.Context, domain.MailScope, func(*domain.State) error) error
	MailMessage(context.Context, string, bool) (domain.Message, error)
	MailPage(context.Context, bool, domain.MailQuery) (domain.MailPage, error)
	MailThread(context.Context, bool, string, domain.MailQuery) (domain.MessagePage, error)
	MailStats(context.Context, bool) (domain.MailStats, error)
	OwnsAttachment(context.Context, string, bool) (bool, error)
	PendingMail(context.Context, int, int) ([]string, error)
	PatchThread(context.Context, bool, string, string, domain.MailPatch) error
}

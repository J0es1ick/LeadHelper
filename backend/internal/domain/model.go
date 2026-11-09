// Package domain contains framework-independent business entities.
package domain

import "time"

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	Source      string     `json:"source"`
	ExternalID  string     `json:"externalId,omitempty"`
	URL         string     `json:"url,omitempty"`
	Project     string     `json:"project"`
	AssigneeID  string     `json:"assigneeId"`
	Estimate    float64    `json:"estimate"`
	DueDate     string     `json:"dueDate"`
	PlannedDate string     `json:"plannedDate"`
	Personal    bool       `json:"personal"`
	Tags        []string   `json:"tags"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Demo        bool       `json:"demo"`
}
type Member struct {
	Email       string           `json:"email,omitempty"`
	JiraUserID  string           `json:"jiraUserId,omitempty"`
	JiraLogin   string           `json:"jiraLogin,omitempty"`
	JiraName    string           `json:"jiraName,omitempty"`
	JiraSite    string           `json:"jiraSite,omitempty"`
	Source      string           `json:"source,omitempty"`
	ExternalID  string           `json:"externalId,omitempty"`
	AccountID   string           `json:"accountId,omitempty"`
	Inactive    bool             `json:"inactive,omitempty"`
	Departments []TeamDepartment `json:"departments,omitempty"`
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Role        string           `json:"role"`
	Color       string           `json:"color"`
	Capacity    float64          `json:"capacity"`
	Demo        bool             `json:"demo"`
}
type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Tag       string    `json:"tag"`
	Pinned    bool      `json:"pinned"`
	UpdatedAt time.Time `json:"updatedAt"`
	Demo      bool      `json:"demo"`
}
type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Data        string `json:"data"`
	BlobKey     string `json:"blobKey,omitempty"`
	Size        int64  `json:"size,omitempty"`
	ContentID   string `json:"contentId,omitempty"`
	Inline      bool   `json:"inline,omitempty"`
}
type Message struct {
	ID            string       `json:"id"`
	ExternalID    string       `json:"externalId,omitempty"`
	From          string       `json:"from"`
	FromName      string       `json:"fromName"`
	To            string       `json:"to"`
	CC            string       `json:"cc"`
	Subject       string       `json:"subject"`
	Body          string       `json:"body"`
	HTML          string       `json:"html,omitempty"`
	InReplyTo     string       `json:"inReplyTo,omitempty"`
	References    string       `json:"references,omitempty"`
	FormatVersion int          `json:"formatVersion,omitempty"`
	Folder        string       `json:"folder"`
	Read          bool         `json:"read"`
	Starred       bool         `json:"starred"`
	Date          time.Time    `json:"date"`
	Attachments   []Attachment `json:"attachments"`
	Delivery      string       `json:"delivery,omitempty"`
	Demo          bool         `json:"demo"`
	AccountID     string       `json:"accountId,omitempty"`
	SourceFolder  string       `json:"sourceFolder,omitempty"`
	UIDValidity   uint32       `json:"uidValidity,omitempty"`
	UID           uint32       `json:"uid,omitempty"`
	MessageID     string       `json:"messageId,omitempty"`
	ContentHash   string       `json:"contentHash,omitempty"`
	RawKey        string       `json:"rawKey,omitempty"`
	ImportWarning string       `json:"importWarning,omitempty"`
	Raw           []byte       `json:"-"`
}
type Settings struct {
	Name      string          `json:"name"`
	Workspace string          `json:"workspace"`
	Email     string          `json:"email"`
	Demo      bool            `json:"demo"`
	Modules   map[string]bool `json:"modules"`
}
type SyncLog struct {
	ID       string    `json:"id"`
	Provider string    `json:"provider"`
	At       time.Time `json:"at"`
	Success  bool      `json:"success"`
	Count    int       `json:"count"`
	Message  string    `json:"message"`
	Demo     bool      `json:"demo"`
}
type State struct {
	Revision            string                    `json:"revision,omitempty"`
	TaskMetrics         *TaskMetrics              `json:"taskMetrics,omitempty"`
	TeamPlan            []TeamPlanEntry           `json:"teamPlan,omitempty"`
	TeamPlanVersion     int                       `json:"teamPlanVersion,omitempty"`
	TeamSync            TeamSyncStatus            `json:"teamSync"`
	Version             int                       `json:"version"`
	Settings            Settings                  `json:"settings"`
	Tasks               []Task                    `json:"tasks"`
	Members             []Member                  `json:"members"`
	Notes               []Note                    `json:"notes"`
	Messages            []Message                 `json:"messages"`
	SyncLogs            []SyncLog                 `json:"syncLogs"`
	MailCheckpoints     map[string]MailCheckpoint `json:"mailCheckpoints,omitempty"`
	MailSync            MailSyncStatus            `json:"mailSync"`
	MailStats           *MailStats                `json:"mailStats,omitempty"`
	MailConnection      *Connection               `json:"mailConnection,omitempty"`
	Notifications       []Notification            `json:"notifications"`
	NotificationCursors map[string]string         `json:"notificationCursors,omitempty"`
}

// A checkpoint is committed together with its messages, never before them.
type MailCheckpoint struct {
	AccountID   string    `json:"accountId"`
	Folder      string    `json:"folder"`
	UIDValidity uint32    `json:"uidValidity"`
	LastUID     uint32    `json:"lastUid"`
	SyncedAt    time.Time `json:"syncedAt"`
}
type MailSyncStatus struct {
	AccountID     string     `json:"accountId,omitempty"`
	State         string     `json:"state"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	LastSuccessAt *time.Time `json:"lastSuccessAt,omitempty"`
	UpdatedAt     *time.Time `json:"updatedAt,omitempty"`
	Folder        string     `json:"folder,omitempty"`
	Imported      int        `json:"imported"`
	Error         string     `json:"error,omitempty"`
}
type MailBatch struct {
	Key        string
	Checkpoint MailCheckpoint
	Messages   []Message
}
type Connection struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Configured     bool   `json:"configured"`
	Hint           string `json:"hint"`
	Email          string `json:"email,omitempty"`
	Host           string `json:"host,omitempty"`
	SendConfigured bool   `json:"sendConfigured"`
}
type Workload struct {
	Member  Member  `json:"member"`
	Hours   float64 `json:"hours"`
	Tasks   int     `json:"tasks"`
	Percent float64 `json:"percent"`
}
type Report struct {
	From         string     `json:"from"`
	To           string     `json:"to"`
	Total        int        `json:"total"`
	Completed    int        `json:"completed"`
	InProgress   int        `json:"inProgress"`
	Overdue      int        `json:"overdue"`
	PlannedHours float64    `json:"plannedHours"`
	Workload     []Workload `json:"workload"`
	Tasks        []Task     `json:"tasks"`
}

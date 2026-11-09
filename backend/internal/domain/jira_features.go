package domain

import "time"

type JiraOption struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Key     string `json:"key,omitempty"`
	Subtask bool   `json:"subtask,omitempty"`
}
type JiraCreateField struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Required bool             `json:"required"`
	Schema   map[string]any   `json:"schema"`
	Allowed  []map[string]any `json:"allowedValues,omitempty"`
	Default  any              `json:"defaultValue,omitempty"`
}
type JiraCreateMeta struct {
	Fields []JiraCreateField `json:"fields"`
	Cloud  bool              `json:"cloud"`
	URL    string            `json:"url"`
}
type JiraUpload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}
type JiraCreateInput struct {
	RequestID   string         `json:"requestId"`
	Project     string         `json:"project"`
	IssueType   string         `json:"issueType"`
	Fields      map[string]any `json:"fields"`
	Attachments []JiraUpload   `json:"attachments,omitempty"`
}
type JiraCreateResult struct {
	Task     Task     `json:"task"`
	Warnings []string `json:"warnings"`
}

// A definitive rejection is safe to correct. Other write failures may have committed remotely.
type JiraWriteError struct {
	Message    string
	Definitive bool
}

func (e *JiraWriteError) Error() string { return e.Message }

type Worklog struct {
	ID       string    `json:"id"`
	MemberID string    `json:"memberId"`
	IssueKey string    `json:"issueKey"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	Date     string    `json:"date"`
	Started  time.Time `json:"started"`
	Seconds  int64     `json:"seconds"`
	Comment  string    `json:"comment,omitempty"`
}
type WorklogCell struct {
	MemberID  string        `json:"memberId"`
	Date      string        `json:"date"`
	Seconds   int64         `json:"seconds"`
	Count     int           `json:"count"`
	TaskCount int           `json:"taskCount"`
	Tasks     []WorklogTask `json:"tasks"`
}
type WorklogTask struct {
	IssueKey string `json:"issueKey"`
	Title    string `json:"title"`
	Seconds  int64  `json:"seconds"`
	Count    int    `json:"count"`
}
type WorklogWeek struct {
	Cells    []WorklogCell `json:"cells"`
	SyncedAt *time.Time    `json:"syncedAt,omitempty"`
	Warning  string        `json:"warning,omitempty"`
	Timezone string        `json:"timezone"`
}
type WorklogPage struct {
	Items []Worklog `json:"items"`
	Total int       `json:"total"`
	Page  int       `json:"page"`
	Pages int       `json:"pages"`
	Limit int       `json:"limit"`
}
type JiraWriteRecord struct {
	Hash   string
	Status string
	Result JiraCreateResult
}

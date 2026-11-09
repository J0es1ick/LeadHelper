package application

import (
	"context"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"testing"
	"time"
)

type jiraFeatureFake struct {
	creates      int
	uploads      int
	failure      error
	rows         []domain.Worklog
	worklogError error
}

func (p *jiraFeatureFake) Configured() bool { return true }
func (p *jiraFeatureFake) SiteID() string   { return "site" }
func (p *jiraFeatureFake) FetchTasks(context.Context) ([]domain.Task, []domain.Member, error) {
	return nil, nil, nil
}
func (p *jiraFeatureFake) CreationProjects(context.Context) ([]domain.JiraOption, error) {
	return nil, nil
}
func (p *jiraFeatureFake) CreationTypes(context.Context, string) ([]domain.JiraOption, error) {
	return nil, nil
}
func (p *jiraFeatureFake) CreationFields(context.Context, string, string) (domain.JiraCreateMeta, error) {
	return domain.JiraCreateMeta{}, nil
}
func (p *jiraFeatureFake) AssignableUsers(context.Context, string, string) ([]domain.JiraUser, error) {
	return nil, nil
}
func (p *jiraFeatureFake) CreateIssue(_ context.Context, in domain.JiraCreateInput) (domain.JiraCreateResult, error) {
	p.creates++
	return domain.JiraCreateResult{Task: domain.Task{ID: "jira-TEST-1", ExternalID: "TEST-1", Source: "jira", Title: in.Fields["summary"].(string), Status: "todo", Priority: "medium"}, Warnings: []string{}}, p.failure
}
func (p *jiraFeatureFake) UploadIssueFiles(context.Context, string, []domain.JiraUpload) []string {
	p.uploads++
	return []string{}
}
func (p *jiraFeatureFake) FetchWorklogs(context.Context, []domain.Member, string, string) ([]domain.Worklog, error) {
	return p.rows, p.worklogError
}

func TestCreateJiraJournalPreventsDuplicatesAndAmbiguousRetries(t *testing.T) {
	s, repo, _ := pagedFixture(t, 0)
	ctx := context.Background()
	p := &jiraFeatureFake{}
	s.sources = map[string]ports.TaskSource{"jira": p}
	if e := repo.UpdateCore(ctx, func(st *domain.State) error {
		st.Members[0].JiraSite = "site"
		st.Members[0].JiraUserID = "key-a"
		st.Members[0].JiraLogin = "anna"
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	in := domain.JiraCreateInput{RequestID: "request-1234567890", Project: "100", IssueType: "1", Fields: map[string]any{"summary": "Real creation only in fake", "assignee": map[string]any{"name": "anna"}}}
	first, e := s.CreateJiraIssue(ctx, in)
	if e != nil || first.Task.AssigneeID != "alice" {
		t.Fatal(first, e)
	}
	// A fresh service reuses durable operation records, not just an in-memory lock.
	s = New(repo, map[string]ports.TaskSource{"jira": p}, nil)
	second, e := s.CreateJiraIssue(ctx, in)
	if e != nil || p.creates != 1 || p.uploads != 1 || second.Task.ID != first.Task.ID {
		t.Fatal(p.creates, second, e)
	}
	copy := in
	copy.Fields = map[string]any{"summary": "Different payload"}
	if _, e = s.CreateJiraIssue(ctx, copy); e == nil || p.creates != 1 {
		t.Fatal("request ID reused for different payload")
	}
	in.RequestID = "uncertain-1234567890"
	p.failure = fmt.Errorf("connection disappeared")
	if _, e = s.CreateJiraIssue(ctx, in); !errors.Is(e, ErrJiraUncertain) {
		t.Fatal(e)
	}
	p.failure = nil
	if _, e = s.CreateJiraIssue(ctx, in); !errors.Is(e, ErrJiraUncertain) || p.creates != 2 {
		t.Fatal("ambiguous create retried", p.creates, e)
	}
	in.RequestID = "rejected-1234567890"
	p.failure = &domain.JiraWriteError{Message: "field invalid", Definitive: true}
	if _, e = s.CreateJiraIssue(ctx, in); e == nil {
		t.Fatal("validation ignored")
	}
	p.failure = nil
	if _, e = s.CreateJiraIssue(ctx, in); e != nil || p.creates != 4 {
		t.Fatal("definitive rejection not retryable", p.creates, e)
	}
}
func TestWorklogSnapshotReplacesEditsDeletesAndKeepsPreviousOnFailure(t *testing.T) {
	s, repo, _ := pagedFixture(t, 0)
	ctx := context.Background()
	p := &jiraFeatureFake{}
	s.sources = map[string]ports.TaskSource{"jira": p}
	if e := repo.UpdateCore(ctx, func(st *domain.State) error {
		st.Members[0].JiraSite = "site"
		st.Members[0].JiraUserID = "key-a"
		st.Members[0].JiraLogin = "anna"
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	p.rows = []domain.Worklog{{ID: "1", MemberID: "alice", IssueKey: "DONE-1", Date: "2026-11-09", Seconds: 3600, Started: time.Now()}, {ID: "2", MemberID: "alice", IssueKey: "OTHER-2", Date: "2026-11-10", Seconds: 1800, Started: time.Now()}}
	v, e := s.SyncJiraWorklogs(ctx, "2026-11-09", "2026-11-15", true)
	if e != nil || len(v.Cells) != 2 || v.SyncedAt == nil || v.Warning == "" {
		t.Fatal(v, e)
	}
	for i := 0; i < 2; i++ {
		v, e = s.SyncJiraWorklogs(ctx, "2026-11-09", "2026-11-15", true)
		if e != nil || v.Cells[0].Seconds != 3600 {
			t.Fatal("duplicate sync", v, e)
		}
	}
	p.worklogError = fmt.Errorf("partial page failure")
	p.rows = nil
	if _, e = s.SyncJiraWorklogs(ctx, "2026-11-09", "2026-11-15", true); e == nil {
		t.Fatal("partial error ignored")
	}
	v, e = s.JiraWorklogWeek(ctx, "2026-11-09", "2026-11-15")
	if e != nil || len(v.Cells) != 2 {
		t.Fatal("snapshot lost", v, e)
	}
	p.worklogError = nil
	p.rows = []domain.Worklog{{ID: "1", MemberID: "alice", IssueKey: "DONE-1", Date: "2026-11-10", Seconds: 5400}}
	v, e = s.SyncJiraWorklogs(ctx, "2026-11-09", "2026-11-15", true)
	if e != nil || len(v.Cells) != 1 || v.Cells[0].Seconds != 5400 || v.Cells[0].Date != "2026-11-10" {
		t.Fatal("edit/delete not reflected", v, e)
	}
	day, e := s.JiraWorklogDay(ctx, "alice", "2026-11-10", 999, 25)
	if e != nil || day.Total != 1 || day.Page != 1 || day.Items[0].IssueKey != "DONE-1" {
		t.Fatal(day, e)
	}
	if e = repo.UpdateCore(ctx, func(st *domain.State) error { st.Members[0].JiraUserID = "new-user"; return nil }); e != nil {
		t.Fatal(e)
	}
	v, e = s.JiraWorklogWeek(ctx, "2026-11-09", "2026-11-15")
	if e != nil || len(v.Cells) != 0 || v.SyncedAt != nil {
		t.Fatal("old identity's cache visible", v, e)
	}
}

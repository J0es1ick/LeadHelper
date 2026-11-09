package application

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"testing"
	"time"
)

type jiraTeamFixture struct {
	users   []domain.JiraUser
	fetches int
	fail    bool
	after   func()
}

func (*jiraTeamFixture) Configured() bool { return true }
func (*jiraTeamFixture) TeamScoped() bool { return true }
func (*jiraTeamFixture) SiteID() string   { return "jira-test" }
func (*jiraTeamFixture) FetchTasks(context.Context) ([]domain.Task, []domain.Member, error) {
	return nil, nil, errors.New("global fetch forbidden")
}
func (p *jiraTeamFixture) SearchUsers(context.Context, string) ([]domain.JiraUser, error) {
	return p.users, nil
}
func (p *jiraTeamFixture) User(_ context.Context, id string) (domain.JiraUser, error) {
	for _, u := range p.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.JiraUser{}, errors.New("unknown user")
}
func (p *jiraTeamFixture) FetchTeamTasks(_ context.Context, m []domain.Member) ([]domain.Task, error) {
	p.fetches++
	if p.fail {
		return nil, errors.New("Jira unavailable")
	}
	if p.after != nil {
		p.after()
	}
	return []domain.Task{{ID: "jira-X-1", ExternalID: "X-1", Title: "Team task", Source: "jira", AssigneeID: m[0].ID, Status: "todo", Priority: "medium"}}, nil
}
func TestJiraTeamSyncLinksEmailsPreservesCapacityAndLocalPlan(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	now := time.Now()
	p := &jiraTeamFixture{users: []domain.JiraUser{{ID: "KEY-A", Login: "anna", Name: "Anna", Email: "ANNA@example.test"}}}
	s.sources = map[string]ports.TaskSource{"jira": p}
	repo.Update(ctx, func(st *domain.State) error {
		st.Settings.Demo = false
		st.TeamSync = domain.TeamSyncStatus{AccountID: "bitrix", LastSuccessAt: &now}
		st.Members = []domain.Member{{ID: "bitrix-a", Source: "bitrix", AccountID: "bitrix", Name: "Anna", Email: "anna@example.test", Capacity: 27}, {ID: "jira-KEY-A", Role: "Jira", Capacity: 32}}
		st.Tasks = []domain.Task{{ID: "old-local-id", ExternalID: "X-1", Source: "jira", AssigneeID: "jira-KEY-A", PlannedDate: "2026-11-09"}, {ID: "local", Source: "local", AssigneeID: "jira-KEY-A"}}
		st.TeamPlanVersion = 1
		st.TeamPlan = []domain.TeamPlanEntry{{TaskID: "old-local-id", MemberID: "bitrix-a", Date: "2026-11-09", Hours: 2}, {TaskID: "old-local-id", MemberID: "jira-KEY-A", Date: "2026-11-10", Hours: 3}}
		return nil
	})
	log, e := s.Sync(ctx, "jira")
	if e != nil || !log.Success {
		t.Fatal(log, e)
	}
	st, _ := s.State(ctx)
	if len(st.Members) != 2 || st.Members[0].Capacity != 27 || st.Members[0].JiraUserID != "KEY-A" || !st.Members[1].Inactive {
		t.Fatal("identity/capacity mismatch", st.Members)
	}
	for _, task := range st.Tasks {
		if task.AssigneeID != "bitrix-a" {
			t.Fatal("legacy assignee not merged", task)
		}
	}
	if st.Tasks[0].ID != "old-local-id" || st.Tasks[0].PlannedDate != "2026-11-09" {
		t.Fatal("plan overwritten", st.Tasks)
	}
	if len(st.TeamPlan) != 2 || st.TeamPlan[0].Hours != 2 || st.TeamPlan[1].Date != "2026-11-10" || st.TeamPlan[1].MemberID != "bitrix-a" {
		t.Fatal("day allocations overwritten", st.TeamPlan)
	}
	s.Sync(ctx, "jira")
	st, _ = s.State(ctx)
	if len(st.Tasks) != 2 || len(st.Members) != 2 {
		t.Fatal("duplicates")
	}
	p.fail = true
	log, e = s.Sync(ctx, "jira")
	if e != nil || log.Success {
		t.Fatal(log, e)
	}
	st, _ = s.State(ctx)
	if len(st.Tasks) != 2 {
		t.Fatal("failure wiped tasks")
	}
	m := st.Members[0]
	m.JiraUserID = "forged"
	saved, e := s.SaveMember(ctx, m.ID, m)
	if e != nil || saved.JiraUserID != "KEY-A" {
		t.Fatal("forged binding", saved, e)
	}
}
func TestJiraTeamUnmatchedAndAmbiguousNeverFetchTasks(t *testing.T) {
	for _, users := range [][]domain.JiraUser{nil, {{ID: "A", Email: "same@test"}, {ID: "B", Email: "same@test"}}, {{ID: "wrong", Name: "Same Name", Email: "other@test"}}} {
		s, repo := fixture(t)
		ctx := context.Background()
		now := time.Now()
		p := &jiraTeamFixture{users: users}
		s.sources = map[string]ports.TaskSource{"jira": p}
		repo.Update(ctx, func(st *domain.State) error {
			st.Settings.Demo = false
			st.TeamSync = domain.TeamSyncStatus{AccountID: "b", LastSuccessAt: &now}
			st.Members = []domain.Member{{ID: "a", Source: "bitrix", AccountID: "b", Name: "Same Name", Email: "same@test"}}
			st.Tasks = []domain.Task{{ID: "keep", Source: "jira"}}
			return nil
		})
		log, e := s.Sync(ctx, "jira")
		if e != nil || log.Success || p.fetches != 0 {
			t.Fatal("unmatched team fetched", log, e, p.fetches)
		}
		st, _ := s.State(ctx)
		if len(st.Tasks) != 1 {
			t.Fatal("unmatched cleared tasks")
		}
	}
}
func TestJiraManualLinkValidatesDuplicateAndRosterRace(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	now := time.Now()
	p := &jiraTeamFixture{users: []domain.JiraUser{{ID: "A", Login: "a", Name: "Different name"}}}
	s.sources = map[string]ports.TaskSource{"jira": p}
	repo.Update(ctx, func(st *domain.State) error {
		st.Settings.Demo = false
		st.TeamSync = domain.TeamSyncStatus{AccountID: "b", LastSuccessAt: &now}
		st.Members = []domain.Member{{ID: "one", Source: "bitrix", AccountID: "b"}, {ID: "two", Source: "bitrix", AccountID: "b"}}
		return nil
	})
	if _, e := s.LinkJiraMember(ctx, "one", "A"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.LinkJiraMember(ctx, "two", "A"); e == nil {
		t.Fatal("duplicate binding accepted")
	}
	if _, e := s.LinkJiraMember(ctx, "two", "forged"); e == nil {
		t.Fatal("unknown identity accepted")
	}
	repo.Update(ctx, func(st *domain.State) error { st.Members[1].Inactive = true; return nil })
	p.after = func() { repo.Update(ctx, func(st *domain.State) error { st.Members[0].Inactive = true; return nil }) }
	if _, e := s.Sync(ctx, "jira"); e == nil {
		t.Fatal("roster race applied stale tasks")
	}
	st, _ := s.State(ctx)
	if len(st.Tasks) != 0 {
		t.Fatal("stale tasks applied")
	}
}

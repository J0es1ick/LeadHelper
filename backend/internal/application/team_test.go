package application

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"testing"
)

type teamFixture struct {
	snapshot domain.TeamSnapshot
	err      error
	calls    int
	after    func()
}

func (p *teamFixture) Configured() bool  { return true }
func (p *teamFixture) AccountID() string { return "bitrix-test" }
func (p *teamFixture) Browse(context.Context, domain.RemoteQuery) (domain.RemotePage, error) {
	return domain.RemotePage{}, nil
}
func (p *teamFixture) Calendar(context.Context, string, string) (domain.CalendarPage, error) {
	return domain.CalendarPage{}, nil
}
func (p *teamFixture) SendChat(context.Context, string, string) error { return nil }
func (p *teamFixture) FetchTeam(context.Context) (domain.TeamSnapshot, error) {
	p.calls++
	if p.after != nil {
		p.after()
	}
	return p.snapshot, p.err
}
func teamProvider() *teamFixture {
	return &teamFixture{snapshot: domain.TeamSnapshot{AccountID: "bitrix-test", ManagerID: "7", ManagerName: "Manager", Roots: []domain.TeamDepartment{{ID: "2", Name: "Development"}}, Departments: []domain.TeamDepartment{{ID: "2", Name: "Development"}, {ID: "3", Name: "Mobile", Parent: "2"}}, Members: []domain.TeamPerson{{ID: "8", Name: "Anna", Role: "Developer", Departments: []domain.TeamDepartment{{ID: "3", Name: "Mobile"}}}, {ID: "9", Name: "Max", Role: "QA"}}}}
}

func TestReportKeepsFormerMemberTasksWithoutAvailableCapacity(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	err := repo.Update(ctx, func(st *domain.State) error {
		st.Settings.Demo = false
		st.Members = []domain.Member{
			{ID: "current", Name: "Current", Capacity: 32},
			{ID: "former", Name: "Former", Capacity: 40, Inactive: true},
		}
		st.Tasks = []domain.Task{
			{ID: "current-task", AssigneeID: "current", Status: "todo", Source: "local", PlannedDate: "2026-11-09", Estimate: 8},
			{ID: "former-task", AssigneeID: "former", Status: "todo", Source: "local", PlannedDate: "2026-11-09", Estimate: 12},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.Report(ctx, "2026-11-09", "2026-11-15", "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 2 || report.PlannedHours != 20 {
		t.Fatal("former member task history was lost", report)
	}
	if len(report.Workload) != 1 || report.Workload[0].Member.ID != "current" || report.Workload[0].Percent != 25 {
		t.Fatal("capacity includes a former member", report.Workload)
	}
}

func TestTeamSyncKeepsLocalCapacityAndTaskHistory(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	p := teamProvider()
	s.WithWorkspace(p, nil, nil)
	repo.Update(ctx, func(st *domain.State) error { st.Settings.Demo = false; return nil })
	status, e := s.SetTeamAuto(ctx, true)
	if e != nil || status.MemberCount != 2 || !status.Enabled {
		t.Fatal(status, e)
	}
	st, _ := s.State(ctx)
	if len(st.Members) != 2 {
		t.Fatal("wrong roster", len(st.Members))
	}
	anna := st.Members[0]
	anna.Capacity = 21.5
	anna.Name = "Local overwrite"
	anna.Role = "Fake"
	anna.Source = "local"
	anna.Inactive = true
	saved, e := s.SaveMember(ctx, anna.ID, anna)
	if e != nil || saved.Name != "Anna" || saved.Source != "bitrix" || saved.Inactive {
		t.Fatal("provider identity was editable", saved, e)
	}
	task, e := s.SaveTask(ctx, "", domain.Task{Title: "Keep task", Status: "todo", Priority: "medium", AssigneeID: anna.ID})
	if e != nil {
		t.Fatal(e)
	}
	p.snapshot.Members = p.snapshot.Members[1:]
	if _, e = s.SyncTeam(ctx, false); e != nil {
		t.Fatal(e)
	}
	st, _ = s.State(ctx)
	if len(st.Members) != 2 || !st.Members[0].Inactive || st.Members[0].Capacity != 21.5 || len(st.Tasks) != 1 || st.Tasks[0].AssigneeID != anna.ID {
		t.Fatal("removed person or lost history/capacity")
	}
	if _, e = s.SaveTask(ctx, "", domain.Task{Title: "New inactive assignment", Status: "todo", Priority: "medium", AssigneeID: anna.ID}); e == nil {
		t.Fatal("new inactive assignment accepted")
	}
	task.Title = "Historical task can still be edited"
	if _, e = s.SaveTask(ctx, task.ID, task); e != nil {
		t.Fatal(e)
	}
	p.snapshot = teamProvider().snapshot
	p.snapshot.Members[0].Name = "Anna Renamed"
	s.SyncTeam(ctx, false)
	s.SyncTeam(ctx, false)
	st, _ = s.State(ctx)
	if len(st.Members) != 2 || st.Members[0].Inactive || st.Members[0].ID != anna.ID || st.Members[0].Capacity != 21.5 || st.Members[0].Name != "Anna Renamed" {
		t.Fatal("reactivation not idempotent")
	}
	p.err = errors.New("provider unavailable")
	if _, e = s.SyncTeam(ctx, false); e == nil {
		t.Fatal("error hidden")
	}
	st, _ = s.State(ctx)
	if st.Members[0].Inactive || st.TeamSync.Error == "" || st.TeamSync.LastSuccessAt == nil {
		t.Fatal("error wiped previous roster")
	}
	before := p.calls
	if _, e = s.SetTeamAuto(ctx, false); e != nil {
		t.Fatal(e)
	}
	if p.calls != before {
		t.Fatal("pause fetched provider")
	}
	st, _ = s.State(ctx)
	if st.TeamSync.Enabled || len(st.Members) != 2 {
		t.Fatal("pause removed team")
	}
}
func TestTeamSyncDemoAndSettingsRace(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	p := teamProvider()
	s.WithWorkspace(p, nil, nil)
	if _, e := s.SetTeamAuto(ctx, true); e == nil || p.calls != 0 {
		t.Fatal("demo contacted real provider")
	}
	repo.Update(ctx, func(st *domain.State) error { st.Settings.Demo = false; return nil })
	p.after = func() { repo.Update(ctx, func(st *domain.State) error { st.Settings.Demo = true; return nil }) }
	if _, e := s.SetTeamAuto(ctx, true); e == nil {
		t.Fatal("mode change during request accepted")
	}
	st, _ := s.State(ctx)
	if st.TeamSync.Enabled || st.TeamSync.ManagerName != "" {
		t.Fatal("live team leaked into demo")
	}
	raw, _ := repo.View(ctx)
	for _, m := range raw.Members {
		if m.Source == "bitrix" {
			t.Fatal("real roster imported after mode change")
		}
	}
}
func TestTeamSyncEmptyRosterArchivesWithoutDeletingManualPeople(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	p := teamProvider()
	s.WithWorkspace(p, nil, nil)
	repo.Update(ctx, func(st *domain.State) error { st.Settings.Demo = false; return nil })
	manual, e := s.SaveMember(ctx, "", domain.Member{Name: "External colleague", Capacity: 10, Source: "bitrix", ExternalID: "fake", Inactive: true})
	if e != nil || manual.Source != "" || manual.Inactive {
		t.Fatal("manual source spoofing", e)
	}
	s.SetTeamAuto(ctx, true)
	p.snapshot.Members = nil
	_, e = s.SyncTeam(ctx, false)
	if e != nil {
		t.Fatal(e)
	}
	st, _ := s.State(ctx)
	if st.TeamSync.MemberCount != 0 || len(st.Members) != 3 || st.Members[0].Inactive || !st.Members[1].Inactive || !st.Members[2].Inactive {
		t.Fatal("empty team damaged history or manual member")
	}
}

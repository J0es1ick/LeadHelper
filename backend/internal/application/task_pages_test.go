package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/domain"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pagedFixture(t *testing.T, count int) (*Service, *storage.SQLiteRepository, string) {
	t.Helper()
	st := Seed(time.Now())
	st.Settings.Demo = false
	st.Tasks = []domain.Task{}
	st.Members = []domain.Member{{ID: "alice", Name: "Alice", Capacity: 40}, {ID: "bob", Name: "Bob", Capacity: 40}}
	st.Messages = nil
	st.Notes = nil
	st.TeamPlanVersion = 1
	for i := 0; i < count; i++ {
		st.Tasks = append(st.Tasks, domain.Task{ID: fmt.Sprintf("done-%05d", i), Title: fmt.Sprintf("Архив %05d", i), Description: strings.Repeat("Описание ", 100), Status: "done", Source: "jira", AssigneeID: "alice", Priority: "medium", UpdatedAt: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)})
	}
	today, _, _ := moscowWeek()
	st.Tasks = append(st.Tasks, []domain.Task{
		{ID: "todo", Title: "Новый поиск", Description: "Полное описание сохранено", Status: "todo", AssigneeID: "alice", Source: "jira", Priority: "medium", Estimate: 16},
		{ID: "progress", Title: "В работе", Status: "progress", AssigneeID: "alice", Source: "local", Priority: "high", Estimate: 8},
		{ID: "review", Title: "На проверке", Status: "review", AssigneeID: "alice", Source: "local"},
		{ID: "other", Title: "Чужая задача", Status: "todo", AssigneeID: "bob", Source: "local"},
		{ID: "personal", Title: "Личный план", Status: "todo", AssigneeID: "alice", Source: "local", Personal: true, PlannedDate: today},
		{ID: "demo", Title: "Демо", Status: "todo", Source: "local", AssigneeID: "alice", Demo: true},
	}...)
	file := filepath.Join(t.TempDir(), "db.sqlite")
	repo, e := storage.OpenSQLite(file, "", st)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { repo.Close() })
	return New(repo, nil, nil), repo, file
}

func TestTaskPagesLargeArchiveAndShell(t *testing.T) {
	s, repo, file := pagedFixture(t, 10000)
	ctx := context.Background()
	q := domain.TaskQuery{Personal: "false", Assignee: "all", Status: "done", Page: 1, Limit: 25}
	first, e := s.TaskPage(ctx, q)
	if e != nil || first.Total != 10000 || len(first.Items) != 25 || first.Pages != 400 {
		t.Fatalf("page: %+v %v", first, e)
	}
	q.Page = 2
	second, e := s.TaskPage(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	seen := map[string]bool{}
	for _, x := range first.Items {
		if x.Description != "" {
			t.Fatal("description in summary")
		}
		seen[x.ID] = true
	}
	for _, x := range second.Items {
		if seen[x.ID] {
			t.Fatal("page overlap")
		}
	}
	q.Page = 99999
	last, e := s.TaskPage(ctx, q)
	if e != nil || last.Page != 400 {
		t.Fatalf("last page: %d %v", last.Page, e)
	}
	q.Search = "АРХИВ 09999"
	p, e := s.TaskPage(ctx, q)
	if e != nil || p.Total != 1 || p.Items[0].ID != "done-09999" {
		t.Fatalf("Unicode search: %+v %v", p, e)
	}
	st, e := s.WorkspaceState(ctx)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(st)
	if len(st.Tasks) > 30 || len(raw) > 60000 || st.TaskMetrics.Statuses["done"] != 10000 || st.TaskMetrics.PersonalTotal != 1 {
		t.Fatalf("shell tasks=%d bytes=%d metrics=%+v", len(st.Tasks), len(raw), st.TaskMetrics)
	}
	if st.Revision == "" {
		t.Fatal("missing revision")
	}
	rev := st.Revision
	full, e := s.Task(ctx, "todo")
	if e != nil || full.Description != "Полное описание сохранено" {
		t.Fatal("full detail missing", e)
	}
	if e = s.TaskStatus(ctx, "todo", "progress"); e != nil {
		t.Fatal(e)
	}
	full, _ = s.Task(ctx, "todo")
	if full.Description != "Полное описание сохранено" || full.Status != "progress" {
		t.Fatal("status lost data")
	}
	st, _ = s.WorkspaceState(ctx)
	if rev == st.Revision || st.TaskMetrics.Statuses["progress"] != 2 {
		t.Fatal("stale index")
	}
	e = repo.UpdateCore(ctx, func(st *domain.State) error { st.Tasks = nil; return errors.New("abort") })
	if e == nil {
		t.Fatal("expected rollback")
	}
	full, e = s.Task(ctx, "todo")
	if e != nil || full.Title == "" {
		t.Fatal("rollback lost rows")
	}
	if e = s.Delete(ctx, "tasks", "progress"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Task(ctx, "progress"); !errors.Is(e, ErrNotFound) {
		t.Fatal("delete index stale", e)
	}
	if e = repo.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := storage.OpenSQLite(file, "", domain.State{})
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	s = New(reopened, nil, nil)
	full, e = s.Task(ctx, "todo")
	if e != nil || full.Status != "progress" {
		t.Fatal("reopen", e)
	}
	cfg := st.Settings
	cfg.Modules["jira"] = false
	if e = s.Settings(ctx, cfg); e != nil {
		t.Fatal(e)
	}
	q.Search = ""
	q.Page = 1
	p, e = s.TaskPage(ctx, q)
	if e != nil || p.Total != 0 {
		t.Fatal("disabled source leak", e)
	}
	if _, e = s.Task(ctx, "demo"); !errors.Is(e, ErrNotFound) {
		t.Fatal("demo leak", e)
	}
}

func TestTeamDayManualPlanAndConflict(t *testing.T) {
	s, _, _ := pagedFixture(t, 2)
	ctx := context.Background()
	date, _, _ := moscowWeek()
	q := domain.TaskQuery{Page: 1, Limit: 25}
	day, e := s.TeamDay(ctx, "alice", date, q)
	if e != nil || day.Candidates.Total != 2 || len(day.Selected) != 0 {
		t.Fatalf("candidates: %+v %v", day, e)
	}
	input := domain.TeamDayInput{Revision: day.Revision, Entries: []domain.TeamPlanEntry{{TaskID: "todo", Hours: 3}, {TaskID: "progress", Hours: 0}}}
	if e = s.SaveTeamDay(ctx, "alice", date, input); e != nil {
		t.Fatal(e)
	}
	if e = s.SaveTeamDay(ctx, "alice", date, input); !errors.Is(e, ErrConflict) {
		t.Fatal("stale save accepted", e)
	}
	day, _ = s.TeamDay(ctx, "alice", date, q)
	if len(day.Selected) != 2 {
		t.Fatal("selection lost")
	}
	week, e := s.TeamWeek(ctx, date, date)
	if e != nil || len(week.Cells) != 1 || week.Cells[0].Count != 2 || week.Cells[0].Hours != 3 || week.Cells[0].Unestimated != 1 {
		t.Fatalf("manual hours %+v %v", week, e)
	}
	for _, id := range []string{"other", "review", "done-00000", "personal", "demo", "missing"} {
		bad := domain.TeamDayInput{Revision: day.Revision, Entries: []domain.TeamPlanEntry{{TaskID: "todo", Hours: 3}, {TaskID: id}}}
		if e = s.SaveTeamDay(ctx, "alice", date, bad); e == nil {
			t.Fatal("invalid choice accepted", id)
		}
		unchanged, _ := s.TeamDay(ctx, "alice", date, q)
		if unchanged.Revision != day.Revision {
			t.Fatal("partial write")
		}
	}
	tomorrow, _ := time.Parse("2006-01-02", date)
	next := tomorrow.AddDate(0, 0, 1).Format("2006-01-02")
	otherDay, _ := s.TeamDay(ctx, "alice", next, q)
	if e = s.SaveTeamDay(ctx, "alice", next, domain.TeamDayInput{Revision: otherDay.Revision, Entries: []domain.TeamPlanEntry{{TaskID: "todo", Hours: 2}}}); e != nil {
		t.Fatal(e)
	}
	if e = s.TaskStatus(ctx, "todo", "done"); e != nil {
		t.Fatal(e)
	}
	day, _ = s.TeamDay(ctx, "alice", date, q)
	if day.Candidates.Total != 1 || len(day.Selected) != 2 {
		t.Fatal("completed history missing")
	}
	if e = s.SaveTeamDay(ctx, "alice", date, domain.TeamDayInput{Revision: day.Revision, Entries: []domain.TeamPlanEntry{{TaskID: "todo", Hours: 1}}}); e != nil {
		t.Fatal("existing completed plan rejected", e)
	}
	week, _ = s.TeamWeek(ctx, date, next)
	if len(week.Cells) != 2 || week.Cells[0].Hours+week.Cells[1].Hours != 3 {
		t.Fatal("multiple day allocation", week)
	}
	report, e := s.Report(ctx, date, next, "all")
	if e != nil || report.Workload[0].Hours != 3 || report.Workload[0].Tasks != 1 {
		t.Fatal("report workload", report.Workload, e)
	}
	day, _ = s.TeamDay(ctx, "alice", date, q)
	if e = s.SaveTeamDay(ctx, "alice", date, domain.TeamDayInput{Revision: day.Revision}); e != nil {
		t.Fatal(e)
	}
	day, _ = s.TeamDay(ctx, "alice", date, q)
	if len(day.Selected) != 0 {
		t.Fatal("clear failed")
	}
	if _, e = s.TeamDay(ctx, "alice", "", q); e == nil {
		t.Fatal("empty date")
	}
	bob, _ := s.TeamDay(ctx, "bob", date, q)
	if e = s.SaveTeamDay(ctx, "bob", date, domain.TeamDayInput{Revision: bob.Revision, Entries: []domain.TeamPlanEntry{{TaskID: "other", Hours: 25}}}); e == nil {
		t.Fatal("invalid hours")
	}
}

func TestTeamPlanMigrationIdempotentAndTaskEditsPreservePlan(t *testing.T) {
	s, repo, _ := pagedFixture(t, 0)
	ctx := context.Background()
	date, _, _ := moscowWeek()
	if e := repo.UpdateCore(ctx, func(st *domain.State) error { st.TeamPlanVersion = 0; st.Tasks[0].PlannedDate = date; return nil }); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e := s.UpgradeTeamPlans(ctx); e != nil {
			t.Fatal(e)
		}
	}
	day, e := s.TeamDay(ctx, "alice", date, domain.TaskQuery{Limit: 25, Page: 1})
	if e != nil || len(day.Selected) != 1 || day.Selected[0].Hours != 16 {
		t.Fatalf("migration %+v %v", day, e)
	}
	task, _ := s.Task(ctx, "todo")
	task.Title = "Обновлено из источника"
	if _, e = s.SaveTask(ctx, task.ID, task); e != nil {
		t.Fatal(e)
	}
	after, _ := s.TeamDay(ctx, "alice", date, domain.TaskQuery{Limit: 25, Page: 1})
	if after.Revision != day.Revision || after.Selected[0].Task.Title != task.Title {
		t.Fatal("task update lost allocation")
	}
}

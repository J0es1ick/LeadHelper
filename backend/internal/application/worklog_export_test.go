package application

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"sync/atomic"
	"testing"
	"time"
)

type exportSource struct {
	jiraFeatureFake
	fetch func(context.Context, []domain.Member, string, string) ([]domain.Worklog, error)
}

func (p *exportSource) FetchWorklogs(ctx context.Context, m []domain.Member, from, to string) ([]domain.Worklog, error) {
	return p.fetch(ctx, m, from, to)
}
func exportFixture(t *testing.T, fetch func(context.Context, []domain.Member, string, string) ([]domain.Worklog, error)) *Service {
	t.Helper()
	s, repo, _ := pagedFixture(t, 0)
	s.sources = map[string]ports.TaskSource{"jira": &exportSource{fetch: fetch}}
	if e := repo.UpdateCore(context.Background(), func(st *domain.State) error {
		st.Settings.Modules["team"] = true
		st.Settings.Modules["jira"] = true
		st.Members = []domain.Member{
			{ID: "alice", Name: "Иванова Анна", JiraSite: "site", JiraUserID: "a", JiraLogin: "anna"},
			{ID: "zero", Name: "Сидоров Борис", JiraSite: "site", JiraUserID: "b", JiraLogin: "boris"},
			{ID: "missing", Name: "Петров Игорь"},
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.StopExports)
	return s
}
func waitExport(t *testing.T, s *Service, id string) (domain.WorklogExportStatus, domain.WorklogExportReport) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, report, e := s.WorklogExport(id)
		if e != nil {
			t.Fatal(e)
		}
		if status.State != "running" {
			return status, report
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("export stalled")
	return domain.WorklogExportStatus{}, domain.WorklogExportReport{}
}
func TestWorklogExportIndependentTotalsZeroAndUnavailable(t *testing.T) {
	s := exportFixture(t, func(ctx context.Context, m []domain.Member, from, to string) ([]domain.Worklog, error) {
		if len(m) != 2 {
			t.Error("unmapped employee sent to Jira", m)
		}
		return []domain.Worklog{
			{ID: "1", MemberID: "alice", Date: "2026-11-09", Seconds: 7200},
			{ID: "2", MemberID: "alice", Date: "2026-11-11", Seconds: 1800},
			{ID: "3", MemberID: "alice", Date: "2026-11-12", Seconds: 10800},
			{ID: "4", MemberID: "alice", Date: "2026-11-15", Seconds: 60},
			{ID: "4", MemberID: "alice", Date: "2026-11-15", Seconds: 60},
			{ID: "outside", MemberID: "alice", Date: "2026-11-16", Seconds: 9999},
			{ID: "stranger", MemberID: "unselected", Date: "2026-11-09", Seconds: 9999},
		}, nil
	})
	st, e := s.StartWorklogExport(context.Background(), domain.WorklogExportRequest{From: "2026-11-09", To: "2026-11-15", Split: "weekly", Weekdays: []int{1, 4}, Boundary: "start", MemberIDs: []string{"alice", "missing", "zero"}})
	if e != nil {
		t.Fatal(e)
	}
	st, r := waitExport(t, s, st.ID)
	if st.State != "ready" || r.Missing != 1 || r.Total != 19860 || r.Records != 4 || len(r.Rows) != 3 {
		t.Fatal(st, r)
	}
	for _, row := range r.Rows {
		switch row.MemberID {
		case "alice":
			if row.Seconds[0] != 9000 || row.Seconds[1] != 10860 || row.Total != 19860 {
				t.Fatal("cumulative/overlapping periods", row)
			}
		case "zero":
			if !row.Available || row.Total != 0 {
				t.Fatal("known zero lost", row)
			}
		case "missing":
			if row.Available {
				t.Fatal("missing data became zero", row)
			}
		}
	}
}
func TestWorklogExportChunksAndSourceFailure(t *testing.T) {
	var calls atomic.Int32
	s := exportFixture(t, func(ctx context.Context, m []domain.Member, from, to string) ([]domain.Worklog, error) {
		a, _ := time.Parse("2006-01-02", from)
		b, _ := time.Parse("2006-01-02", to)
		if b.Sub(a) > 30*24*time.Hour {
			t.Error("unbounded source range")
		}
		calls.Add(1)
		return []domain.Worklog{{ID: from, MemberID: "alice", Date: from, Seconds: 3600}}, nil
	})
	in := domain.WorklogExportRequest{From: "2026-01-01", To: "2026-03-10", MemberIDs: []string{"alice"}}
	st, e := s.StartWorklogExport(context.Background(), in)
	if e != nil {
		t.Fatal(e)
	}
	st, r := waitExport(t, s, st.ID)
	if st.State != "ready" || st.Done != 3 || st.Chunks != 3 || calls.Load() != 3 || r.Total != 10800 {
		t.Fatal(st, r, calls.Load())
	}
	calls.Store(0)
	s.sources["jira"].(*exportSource).fetch = func(ctx context.Context, m []domain.Member, from, to string) ([]domain.Worklog, error) {
		if calls.Add(1) == 2 {
			return nil, errors.New("Jira unavailable")
		}
		return []domain.Worklog{{ID: from, MemberID: "alice", Date: from, Seconds: 3600}}, nil
	}
	st, e = s.StartWorklogExport(context.Background(), in)
	if e != nil {
		t.Fatal(e)
	}
	st, r = waitExport(t, s, st.ID)
	if st.State != "error" || st.Done != 1 || len(r.Rows) != 0 {
		t.Fatal("partial report returned", st, r)
	}
}
func TestWorklogExportCancellationAndRosterValidation(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	s := exportFixture(t, func(ctx context.Context, m []domain.Member, from, to string) ([]domain.Worklog, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	})
	in := domain.WorklogExportRequest{From: "2026-11-09", To: "2026-11-15", MemberIDs: []string{"alice"}}
	invalid := in
	invalid.MemberIDs = []string{"unknown"}
	if _, e := s.StartWorklogExport(context.Background(), invalid); e == nil {
		t.Fatal("unknown member accepted")
	}
	invalid.MemberIDs = []string{"missing"}
	if _, e := s.StartWorklogExport(context.Background(), invalid); e == nil {
		t.Fatal("empty mapping accepted")
	}
	st, e := s.StartWorklogExport(context.Background(), in)
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("source not called")
	}
	if _, e = s.StartWorklogExport(context.Background(), in); e == nil {
		t.Fatal("concurrent exports were allowed")
	}
	if e = s.CancelWorklogExport(st.ID); e != nil {
		t.Fatal(e)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("source not cancelled")
	}
	st, r := waitExport(t, s, st.ID)
	if st.State != "cancelled" || len(r.Rows) != 0 {
		t.Fatal(st, r)
	}
}

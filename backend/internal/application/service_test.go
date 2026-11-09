package application

import (
	"context"
	"errors"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Service, *storage.JSONRepository) {
	t.Helper()
	repo, e := storage.Open(filepath.Join(t.TempDir(), "state.json"), Seed(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)))
	if e != nil {
		t.Fatal(e)
	}
	return New(repo, nil, nil), repo
}
func TestTasksPersistAcrossReopenAndModeSwitch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db.json")
	repo, e := storage.Open(path, Seed(time.Now()))
	if e != nil {
		t.Fatal(e)
	}
	s := New(repo, nil, nil)
	task, e := s.SaveTask(ctx, "", domain.Task{Title: "Persistence test", Status: "todo", Priority: "high", Estimate: 3})
	if e != nil {
		t.Fatal(e)
	}
	reopened, e := storage.Open(path, domain.State{})
	if e != nil {
		t.Fatal(e)
	}
	s = New(reopened, nil, nil)
	st, e := s.State(ctx)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, v := range st.Tasks {
		if v.ID == task.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("saved task lost")
	}
	cfg := st.Settings
	cfg.Demo = false
	if e = s.Settings(ctx, cfg); e != nil {
		t.Fatal(e)
	}
	st, _ = s.State(ctx)
	if len(st.Tasks) != 0 || len(st.Notes) != 0 || len(st.Messages) != 0 || len(st.Members) != 0 {
		t.Fatal("demo records leaked into live mode")
	}
	if _, e = s.SaveTask(ctx, task.ID, task); !errors.Is(e, ErrNotFound) {
		t.Fatalf("cross-mode write: %v", e)
	}
	cfg.Demo = true
	if e = s.Settings(ctx, cfg); e != nil {
		t.Fatal(e)
	}
	st, _ = s.State(ctx)
	if len(st.Tasks) == 0 {
		t.Fatal("demo data lost")
	}
}
func TestDisabledModulesBlockWritesAndFilterSources(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	st, _ := s.State(ctx)
	cfg := st.Settings
	cfg.Modules["jira"] = false
	cfg.Modules["mail"] = false
	cfg.Modules["notes"] = false
	cfg.Modules["planner"] = false
	if e := s.Settings(ctx, cfg); e != nil {
		t.Fatal(e)
	}
	st, _ = s.State(ctx)
	for _, v := range st.Tasks {
		if v.Source == "jira" || v.Personal {
			t.Fatal("disabled tasks exposed")
		}
	}
	if len(st.Messages) > 0 || len(st.Notes) > 0 {
		t.Fatal("disabled records exposed")
	}
	if _, e := s.SaveDraft(ctx, "", domain.Message{}); !errors.Is(e, ErrDisabled) {
		t.Fatalf("mail not gated: %v", e)
	}
	if _, e := s.SaveNote(ctx, "", domain.Note{Title: "test"}); !errors.Is(e, ErrDisabled) {
		t.Fatalf("notes not gated: %v", e)
	}
	if _, e := s.Sync(ctx, "jira"); !errors.Is(e, ErrDisabled) {
		t.Fatalf("sync not gated: %v", e)
	}
}
func TestTaskValidationAndCompletedDate(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	for _, v := range []domain.Task{{Title: "x", Status: "wrong", Priority: "low"}, {Title: "x", Status: "todo", Priority: "low", Estimate: -2}, {Title: "x", Status: "todo", Priority: "low", DueDate: "2026-02-31"}, {Title: "x", Status: "todo", Priority: "low", AssigneeID: "unknown"}} {
		if _, e := s.SaveTask(ctx, "", v); e == nil {
			t.Fatal("invalid task accepted")
		}
	}
	v, e := s.SaveTask(ctx, "", domain.Task{Title: "x", Status: "done", Priority: "low"})
	if e != nil || v.CompletedAt == nil {
		t.Fatalf("completion missing: %v", e)
	}
	v.Status = "todo"
	v, e = s.SaveTask(ctx, v.ID, v)
	if e != nil || v.CompletedAt != nil {
		t.Fatal("reopened completion must clear")
	}
}
func TestDemoSendIsIdempotentAndNeverCallsProvider(t *testing.T) {
	s, repo := fixture(t)
	p := &mockMail{}
	s = New(repo, nil, p)
	ctx := context.Background()
	draft, e := s.SaveDraft(ctx, "", domain.Message{To: "test@example.test", Subject: "Hello", Body: "Test"})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.Send(ctx, draft.ID); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	st, _ := s.State(ctx)
	count := 0
	for _, m := range st.Messages {
		if m.ID == draft.ID {
			count++
			if m.Folder != "sent" || m.Delivery != "demo" {
				t.Fatal("wrong delivery state")
			}
		}
	}
	if count != 1 || p.sent != 0 {
		t.Fatal("demo sent externally or duplicated")
	}
}

type mockMail struct{ sent int }

func (*mockMail) ValidateSend(domain.Message) error { return nil }

func (*mockMail) Configured() bool                                        { return true }
func (*mockMail) FetchMessages(context.Context) ([]domain.Message, error) { return nil, nil }
func (m *mockMail) Send(context.Context, domain.Message) error            { m.sent++; return nil }

type mockSource struct{ fail bool }

func (*mockSource) Configured() bool { return true }
func (m *mockSource) FetchTasks(context.Context) ([]domain.Task, []domain.Member, error) {
	if m.fail {
		return nil, nil, errors.New("sanitized provider failure")
	}
	return []domain.Task{{ID: "jira-X", ExternalID: "X", Title: "Imported", Status: "todo", Priority: "high", Source: "jira", Tags: []string{}}}, nil, nil
}
func TestSyncUpsertsAndPreservesLocalPlan(t *testing.T) {
	_, repo := fixture(t)
	ctx := context.Background()
	source := &mockSource{}
	s := New(repo, map[string]ports.TaskSource{"jira": source}, nil)
	st, _ := s.State(ctx)
	cfg := st.Settings
	cfg.Demo = false
	_ = s.Settings(ctx, cfg)
	log, e := s.Sync(ctx, "jira")
	if e != nil || !log.Success {
		t.Fatalf("sync failed: %v", e)
	}
	st, _ = s.State(ctx)
	task := st.Tasks[0]
	task.PlannedDate = "2026-11-12"
	_, e = s.SaveTask(ctx, task.ID, task)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Sync(ctx, "jira")
	if e != nil {
		t.Fatal(e)
	}
	st, _ = s.State(ctx)
	if len(st.Tasks) != 1 || st.Tasks[0].PlannedDate != "2026-11-12" {
		t.Fatal("upsert duplicated or erased plan")
	}
	source.fail = true
	log, e = s.Sync(ctx, "jira")
	if e != nil || log.Success {
		t.Fatal("failed provider not logged")
	}
	st, _ = s.State(ctx)
	if len(st.Tasks) != 1 {
		t.Fatal("failed sync changed data")
	}
}
func TestReportPeriodAndPersonalExclusion(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	_ = repo.Update(ctx, func(st *domain.State) error {
		st.Tasks = []domain.Task{{ID: "1", Title: "planned", Status: "todo", Source: "local", PlannedDate: "2026-11-04", Estimate: 4, Demo: true}, {ID: "2", Title: "personal", Status: "todo", Source: "local", PlannedDate: "2026-11-04", Estimate: 50, Personal: true, Demo: true}, {ID: "3", Title: "outside", Status: "todo", Source: "local", PlannedDate: "2026-11-14", Estimate: 100, Demo: true}}
		return nil
	})
	r, e := s.Report(ctx, "2026-11-02", "2026-11-08", "all")
	if e != nil || r.Total != 1 || r.PlannedHours != 4 {
		t.Fatalf("incorrect report: %+v / %v", r, e)
	}
}

package storage

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRollbackIsolationAndConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db.json")
	r, e := Open(path, domain.State{Version: 1, Tasks: []domain.Task{}})
	if e != nil {
		t.Fatal(e)
	}
	_ = r.Update(ctx, func(s *domain.State) error {
		s.Tasks = append(s.Tasks, domain.Task{ID: "rollback"})
		return errors.New("cancel")
	})
	v, _ := r.View(ctx)
	if len(v.Tasks) != 0 {
		t.Fatal("transaction not rolled back")
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := r.Update(ctx, func(s *domain.State) error { s.Tasks = append(s.Tasks, domain.Task{Title: "concurrent"}); return nil }); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	v, _ = r.View(ctx)
	if len(v.Tasks) != 30 {
		t.Fatal("lost writes")
	}
	v.Tasks[0].Title = "mutated"
	v, _ = r.View(ctx)
	if v.Tasks[0].Title == "mutated" {
		t.Fatal("view leaked reference")
	}
	reopened, e := Open(path, domain.State{})
	if e != nil {
		t.Fatal(e)
	}
	v, _ = reopened.View(ctx)
	if len(v.Tasks) != 30 {
		t.Fatal("durability failure")
	}
}
func TestCorruptDataIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.json")
	_ = os.WriteFile(path, []byte("invalid"), 0600)
	if _, e := Open(path, domain.State{Version: 1}); e == nil {
		t.Fatal("corruption silently discarded")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "invalid" {
		t.Fatal("corruption overwritten")
	}
}

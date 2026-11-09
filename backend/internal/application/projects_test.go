package application

import (
	"context"
	"fmt"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/domain"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectsManualImportAtomicityPagingAndMode(t *testing.T) {
	ctx := context.Background()
	seed := Seed(time.Now())
	seed.Settings.Demo = false
	r, e := storage.OpenSQLite(filepath.Join(t.TempDir(), "projects.sqlite"), "", seed)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	s := New(r, nil, nil)
	manual := domain.Project{Name: "Визионер", Status: "Разработка", Owner: "Анна", Description: "Сохранить описание", State: "active"}
	if e = s.SaveProject(ctx, manual); e != nil {
		t.Fatal(e)
	}
	rows, e := s.PreviewProjects(ctx, []domain.Project{{Name: "Визионер", Status: "На согласовании", State: "active", Source: "table.xlsx"}, {Name: "Склад", Status: "Готов", State: "completed"}}, true, []string{"name", "status"})
	if e != nil {
		t.Fatal(e)
	}
	if rows[0].Version != 1 || rows[0].Owner != "Анна" || rows[0].Description != "Сохранить описание" {
		t.Fatal("unmapped fields erased", rows)
	}
	page, _ := s.Projects(ctx, domain.ProjectQuery{State: "all"})
	if page.Total != 1 {
		t.Fatal("preview wrote data")
	}
	if e = s.ImportProjects(ctx, rows, "request-12345"); e != nil {
		t.Fatal(e)
	}
	if e = s.ImportProjects(ctx, rows, "request-12345"); e != nil {
		t.Fatal("retry not idempotent", e)
	}
	page, _ = s.Projects(ctx, domain.ProjectQuery{State: "active", Search: "ВИЗИОНЕР"})
	if page.Total != 1 || page.Items[0].Status != "На согласовании" || page.Counts["completed"] != 0 {
		t.Fatal(page)
	}
	// A conflicting row rolls back the entire batch, including a preceding insert.
	batch := []domain.Project{{ID: "must-rollback", Name: "Не сохранять", State: "active"}, rows[0]}
	if e = s.ImportProjects(ctx, batch, "request-conflict"); e == nil {
		t.Fatal("stale version accepted")
	}
	page, _ = s.Projects(ctx, domain.ProjectQuery{State: "all"})
	if page.Total != 2 {
		t.Fatal("partial import committed")
	}
	for i := 0; i < 30; i++ {
		if e = s.SaveProject(ctx, domain.Project{Name: fmt.Sprintf("Проект %02d", i), State: "active"}); e != nil {
			t.Fatal(e)
		}
	}
	page, _ = s.Projects(ctx, domain.ProjectQuery{State: "all", Page: 2, Limit: 25})
	if page.Total != 32 || len(page.Items) != 7 {
		t.Fatal("paging", page.Total, len(page.Items))
	}
	if _, e = s.PreviewProjects(ctx, []domain.Project{{Name: "A"}, {Name: "a"}}, false, nil); e == nil {
		t.Fatal("duplicate rows accepted")
	}
	if e = s.SaveProject(ctx, domain.Project{Name: "bad", URL: "javascript:alert(1)"}); e == nil {
		t.Fatal("unsafe URL accepted")
	}
	_ = r.UpdateCore(ctx, func(st *domain.State) error { st.Settings.Demo = true; return nil })
	page, _ = s.Projects(ctx, domain.ProjectQuery{State: "all"})
	if page.Total != 0 {
		t.Fatal("work projects exposed in demo")
	}
	if e = s.SaveProject(ctx, domain.Project{ID: rows[0].ID, Name: "cross mode", Version: 2}); e == nil {
		t.Fatal("cross mode mutation accepted")
	}
	_ = r.UpdateCore(ctx, func(st *domain.State) error { st.Settings.Modules["projects"] = false; return nil })
	if _, e = s.Projects(ctx, domain.ProjectQuery{}); e == nil {
		t.Fatal("disabled module exposed")
	}
}

func TestProjectFoldersAndPerSheetImport(t *testing.T) {
	ctx := context.Background()
	seed := Seed(time.Now())
	seed.Settings.Demo = false
	r, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "folders.sqlite"), "", seed)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	s := New(r, nil, nil)
	for _, p := range []domain.Project{
		{Name: "Один проект", Group: "Пресейл", Owner: "Анна", Description: "Сохранить", Status: "Оценка"},
		{Name: "Один проект", Group: "Разработка", Owner: "Иван", Status: "В работе"},
		{Name: "Подпроект", Group: "Пресейл / Партнёры", Status: "В работе"},
		{Name: "Личный проект", Status: "В работе"},
	} {
		if err := s.SaveProject(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	batches := []domain.ProjectImportBatch{
		{Fields: []string{"name", "group", "status", "history"}, Projects: []domain.Project{{Name: "Один проект", Group: "Пресейл", Status: "Согласован", History: []domain.ProjectStatus{{Key: "c2", Label: "01.11", Text: "Согласован"}}}}},
		{Fields: []string{"name", "group", "owner"}, Projects: []domain.Project{{Name: "Один проект", Group: "Разработка", Owner: "Пётр"}}},
	}
	rows, err := s.PreviewProjectBatches(ctx, batches, true)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].ID == rows[1].ID || rows[0].Owner != "Анна" || rows[0].Description != "Сохранить" || rows[1].Status != "В работе" || rows[1].Owner != "Пётр" {
		t.Fatal("folder identity or per-sheet field mask lost", rows)
	}
	if err = s.ImportProjects(ctx, rows, "folders-import-1"); err != nil {
		t.Fatal(err)
	}
	if err = s.ImportProjects(ctx, rows, "folders-import-1"); err != nil {
		t.Fatal("retry", err)
	}
	page, err := s.Projects(ctx, domain.ProjectQuery{State: "all", Folder: "Пресейл"})
	if err != nil || page.Total != 2 || page.Counts["active"] != 2 {
		t.Fatal("folder subtree", page, err)
	}
	if len(page.Items[0].History) > 0 {
		t.Fatal("list includes history payload")
	}
	p, err := s.ProjectDetail(ctx, rows[0].ID)
	if err != nil || len(p.History) != 1 {
		t.Fatal("import history missing", err)
	}
	p.Status = "Правка вручную"
	p.Group = "Новая папка"
	p.History = nil
	if err = s.SaveProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	p, err = s.ProjectDetail(ctx, p.ID)
	if err != nil || p.Group != "Новая папка" || len(p.History) != 1 {
		t.Fatal("manual folder/history edit", p, err)
	}
	page, _ = s.Projects(ctx, domain.ProjectQuery{State: "all", Unfiled: true})
	if page.Total != 1 {
		t.Fatal("unfiled filter", page)
	}
	duplicate := []domain.ProjectImportBatch{{Fields: []string{"name", "group"}, Projects: []domain.Project{{Name: "A", Group: "X"}, {Name: "a", Group: "X"}}}}
	if _, err = s.PreviewProjectBatches(ctx, duplicate, false); err == nil {
		t.Fatal("same-folder duplicate allowed")
	}
	bulk := make([]domain.Project, 320)
	for i := range bulk {
		bulk[i] = domain.Project{Name: fmt.Sprintf("Пакет %d", i), Group: "Большой лист"}
	}
	preview, err := s.PreviewProjectBatches(ctx, []domain.ProjectImportBatch{{Projects: bulk, Fields: []string{"name", "group"}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ImportProjects(ctx, preview, "folders-bulk-320"); err != nil {
		t.Fatal(err)
	}
	page, _ = s.Projects(ctx, domain.ProjectQuery{State: "all", Folder: "Большой лист", Page: 13, Limit: 25})
	if page.Total != 320 || len(page.Items) != 20 {
		t.Fatal("large folder pagination", page.Total, len(page.Items))
	}
}

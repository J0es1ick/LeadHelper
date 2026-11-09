package storage

import (
	"context"
	"leadhelper/internal/domain"
	"testing"
	"time"
)

func TestProjectVisibilityRepairPreservesEditsAndDoesNotReplaySnapshot(t *testing.T) {
	ctx := context.Background()
	r := sqlFixture(t, nil)
	if err := r.Update(ctx, func(st *domain.State) error { st.Settings.Modules["projects"] = true; return nil }); err != nil {
		t.Fatal(err)
	}
	book := domain.ProjectSheetSnapshot{SpreadsheetID: "book", Title: "Projects", FetchedAt: time.Now()}
	projects := []domain.Project{
		{ID: "visible", Name: "Current", State: "active", Status: "Old", Group: "Work", Sheet: &domain.ProjectSheetRef{SpreadsheetID: "book", RowID: "a"}},
		{ID: "row", Name: "Hidden row", State: "active", Status: "Old", Group: "Work", Sheet: &domain.ProjectSheetRef{SpreadsheetID: "book", RowID: "b"}},
		{ID: "sheet", Name: "Hidden sheet", State: "completed", Status: "Done", Group: "Old", Sheet: &domain.ProjectSheetRef{SpreadsheetID: "book", RowID: "c"}},
	}
	if err := r.MergeProjectSheet(ctx, book, projects, true); err != nil {
		t.Fatal(err)
	}
	local, _ := r.ProjectDetail(ctx, false, "row")
	local.Description = "Keep manual edit"
	local.Status = "Local status"
	if err := r.SaveProjects(ctx, false, []domain.Project{local}, ""); err != nil {
		t.Fatal(err)
	}
	book.VisibilityKnown = true
	projects[1].SourceVisibility = "row"
	projects[2].SourceVisibility = "sheet"
	if err := r.MergeProjectSheet(ctx, book, projects, true); err != nil {
		t.Fatal(err)
	}
	page, err := r.Projects(ctx, false, domain.ProjectQuery{State: "all", Page: 1, Limit: 25})
	if err != nil || page.Total != 1 || page.HiddenTotal != 2 || len(page.Groups) != 1 || page.Counts["completed"] != 0 {
		t.Fatal("hidden rows affect default totals", page, err)
	}
	local, _ = r.ProjectDetail(ctx, false, "row")
	if local.Description != "Keep manual edit" || local.Status != "Local status" || local.SyncState != "pending" || local.SourceVisibility != "row" {
		t.Fatal("repair overwrote edit", local)
	}
	version := local.Version
	if err = r.MergeProjectSheet(ctx, book, projects, true); err != nil {
		t.Fatal(err)
	}
	local, _ = r.ProjectDetail(ctx, false, "row")
	if local.Version != version {
		t.Fatal("repair not idempotent")
	}
	all, _ := r.Projects(ctx, false, domain.ProjectQuery{State: "all", Page: 1, Limit: 25, IncludeHidden: true})
	if all.Total != 3 {
		t.Fatal("hidden history lost", all)
	}
	book.VisibilityKnown = false
	projects[1].SourceVisibility = ""
	if err = r.MergeProjectSheet(ctx, book, projects, false); err != nil {
		t.Fatal(err)
	}
	local, _ = r.ProjectDetail(ctx, false, "row")
	if local.SourceVisibility != "row" {
		t.Fatal("legacy payload unhid row")
	}
	book.VisibilityKnown = true
	if err = r.MergeProjectSheet(ctx, book, projects, false); err != nil {
		t.Fatal(err)
	}
	projects[1].SourceVisibility = "row"
	if err = r.MergeProjectSheet(ctx, book, projects, true); err != nil {
		t.Fatal(err)
	}
	local, _ = r.ProjectDetail(ctx, false, "row")
	if local.SourceVisibility != "" || local.Description != "Keep manual edit" {
		t.Fatal("stale bootstrap overrode current source", local)
	}
}

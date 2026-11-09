package storage

import (
	"context"
	"leadhelper/internal/domain"
	"testing"
	"time"
)

func TestProjectSyncMergeConflictAndPaging(t *testing.T) {
	ctx := context.Background()
	r := sqlFixture(t, nil)
	if e := r.Update(ctx, func(st *domain.State) error { st.Settings.Modules["projects"] = true; return nil }); e != nil {
		t.Fatal(e)
	}
	remote := domain.Project{ID: "p", Name: "A", Status: "Old", State: "active", Owner: "Ann", Group: "Group", Sheet: &domain.ProjectSheetRef{SpreadsheetID: "book", RowID: "row", Revision: "r1"}, History: []domain.ProjectStatus{{Key: "c2", Text: "Old"}}}
	book := domain.ProjectSheetSnapshot{SpreadsheetID: "book", Title: "Test", FetchedAt: time.Now()}
	merge := func(p domain.Project) {
		t.Helper()
		if e := r.MergeProjectSheet(ctx, book, []domain.Project{p}, false); e != nil {
			t.Fatal(e)
		}
	}
	merge(remote)
	local, e := r.ProjectDetail(ctx, false, "p")
	if e != nil {
		t.Fatal(e)
	}
	local.Owner = "Bob"
	if e = r.SaveProjects(ctx, false, []domain.Project{local}, ""); e != nil {
		t.Fatal(e)
	}
	remote.Status = "From table"
	merge(remote)
	local, _ = r.ProjectDetail(ctx, false, "p")
	if local.Owner != "Bob" || local.Status != "From table" || local.SyncState != "pending" {
		t.Fatal(local)
	}
	pending, e := r.PendingProjectChanges(ctx, "book")
	if e != nil || len(pending) != 1 || pending[0].Base.Status != "From table" {
		t.Fatal(pending, e)
	}
	local.Status = "My status"
	if e = r.SaveProjects(ctx, false, []domain.Project{local}, ""); e != nil {
		t.Fatal(e)
	}
	remote.Status = "Other status"
	merge(remote)
	local, _ = r.ProjectDetail(ctx, false, "p")
	if local.SyncState != "conflict" || local.Status != "My status" {
		t.Fatal(local)
	}
	merge(remote)
	local, _ = r.ProjectDetail(ctx, false, "p")
	if local.SyncState != "conflict" {
		t.Fatal("conflict forgotten")
	}
	if e = r.ResolveProjectConflict(ctx, "p", local.Version-1, true); e == nil {
		t.Fatal("stale resolve")
	}
	if e = r.ResolveProjectConflict(ctx, "p", local.Version, false); e != nil {
		t.Fatal(e)
	}
	local, _ = r.ProjectDetail(ctx, false, "p")
	if local.SyncState != "pending" {
		t.Fatal(local)
	}
	remote.Status = local.Status
	remote.Owner = local.Owner
	merge(remote)
	local, _ = r.ProjectDetail(ctx, false, "p")
	if local.SyncState != "synced" {
		t.Fatal(local)
	}
	page, e := r.Projects(ctx, false, domain.ProjectQuery{State: "all", Page: 1, Limit: 25, Group: "Group"})
	if e != nil || page.Total != 1 || len(page.Items[0].History) != 0 || page.Items[0].HistoryCount != 1 {
		t.Fatal(page, e)
	}
	if e = r.MergeProjectSheet(ctx, book, nil, false); e != nil {
		t.Fatal(e)
	}
	local, _ = r.ProjectDetail(ctx, false, "p")
	if local.SyncState != "conflict" {
		t.Fatal("deleted source silently removed")
	}
}

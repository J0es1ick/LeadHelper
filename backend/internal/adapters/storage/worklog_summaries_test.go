package storage

import (
	"context"
	"fmt"
	"leadhelper/internal/domain"
	"path/filepath"
	"testing"
	"time"
)

func TestWorklogDayTaskSummariesGroupAndBoundWithoutLosingTotals(t *testing.T) {
	r, e := OpenSQLite(filepath.Join(t.TempDir(), "worklogs.sqlite"), "", domain.State{Version: 1})
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	ctx := context.Background()
	logs := []domain.Worklog{
		{ID: "1", MemberID: "anna", Date: "2026-11-09", IssueKey: "TEAM-1", Title: "Первая задача", Seconds: 3600},
		{ID: "2", MemberID: "anna", Date: "2026-11-09", IssueKey: "TEAM-1", Title: "Первая задача", Seconds: 1800},
		{ID: "3", MemberID: "anna", Date: "2026-11-09", IssueKey: "TEAM-2", Title: "Вторая задача", Seconds: 2700},
		{ID: "4", MemberID: "bob", Date: "2026-11-09", IssueKey: "TEAM-1", Title: "Первая задача", Seconds: 600},
		{ID: "5", MemberID: "anna", Date: "2026-11-10", IssueKey: "TEAM-1", Title: "Первая задача", Seconds: 1200},
	}
	if e = r.ReplaceWorklogs(ctx, "site", "2026-11-09", "2026-11-15", logs, time.Now()); e != nil {
		t.Fatal(e)
	}
	v, e := r.WorklogWeek(ctx, "site", "2026-11-09", "2026-11-15")
	if e != nil {
		t.Fatal(e)
	}
	if len(v.Cells) != 3 {
		t.Fatal(v)
	}
	first := v.Cells[0]
	if first.MemberID != "anna" || first.TaskCount != 2 || first.Count != 3 || first.Seconds != 8100 || len(first.Tasks) != 2 {
		t.Fatal(first)
	}
	if first.Tasks[0].IssueKey != "TEAM-1" || first.Tasks[0].Seconds != 5400 || first.Tasks[0].Count != 2 || first.Tasks[0].Title != "Первая задача" {
		t.Fatal(first.Tasks)
	}
	if v.Cells[1].Tasks[0].Seconds != 600 || v.Cells[2].Tasks[0].Seconds != 1200 {
		t.Fatal("days or employees mixed", v)
	}
	if other, e := r.WorklogWeek(ctx, "another-site", "2026-11-09", "2026-11-15"); e != nil || len(other.Cells) != 0 {
		t.Fatal(other, e)
	}
	logs = nil
	for i := 0; i < 1000; i++ {
		logs = append(logs, domain.Worklog{ID: fmt.Sprint(i), MemberID: "anna", Date: "2026-11-09", IssueKey: fmt.Sprintf("TEAM-%04d", i), Title: "Title", Seconds: 60})
	}
	if e = r.ReplaceWorklogs(ctx, "site", "2026-11-09", "2026-11-15", logs, time.Now()); e != nil {
		t.Fatal(e)
	}
	v, e = r.WorklogWeek(ctx, "site", "2026-11-09", "2026-11-15")
	if e != nil || len(v.Cells) != 1 {
		t.Fatal(v, e)
	}
	first = v.Cells[0]
	if first.Count != 1000 || first.TaskCount != 1000 || first.Seconds != 60000 || len(first.Tasks) != 10 || first.Tasks[0].IssueKey != "TEAM-0000" || first.Tasks[9].IssueKey != "TEAM-0009" {
		t.Fatal("bounded summaries lost exact totals or stable order", first)
	}
	if e = r.ReplaceWorklogs(ctx, "site", "2026-11-09", "2026-11-15", nil, time.Now()); e != nil {
		t.Fatal(e)
	}
	v, e = r.WorklogWeek(ctx, "site", "2026-11-09", "2026-11-15")
	if e != nil || len(v.Cells) != 0 {
		t.Fatal("deleted tasks still in cells", v, e)
	}
}

package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestExportPeriodsPartitionWithoutOverlaps(t *testing.T) {
	in := WorklogExportRequest{From: "2026-11-09", To: "2026-11-15", Split: "weekly", Weekdays: []int{1, 4}, Boundary: "start"}
	got, e := WorklogExportPeriods(in)
	want := []WorklogPeriod{{"2026-11-09", "2026-11-11"}, {"2026-11-12", "2026-11-15"}}
	if e != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, e)
	}
	in.Boundary = "end"
	got, e = WorklogExportPeriods(in)
	want = []WorklogPeriod{{"2026-11-09", "2026-11-09"}, {"2026-11-10", "2026-11-12"}, {"2026-11-13", "2026-11-15"}}
	if e != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, e)
	}
	in = WorklogExportRequest{From: "2024-01-01", To: "2024-12-31", Split: "weekly", Weekdays: []int{1, 4}, Boundary: "start"}
	got, e = WorklogExportPeriods(in)
	if e != nil {
		t.Fatal(e)
	}
	next := in.From
	days := 0
	for _, p := range got {
		if p.From != next {
			t.Fatal("gap or overlap", p, next)
		}
		a, _ := time.Parse("2006-01-02", p.From)
		b, _ := time.Parse("2006-01-02", p.To)
		days += int(b.Sub(a)/(24*time.Hour)) + 1
		next = b.AddDate(0, 0, 1).Format("2006-01-02")
	}
	if days != 366 || next != "2025-01-01" {
		t.Fatal(days, next)
	}
}
func TestExportCustomBoundariesAndValidation(t *testing.T) {
	in := WorklogExportRequest{From: "2026-11-09", To: "2026-11-15", Split: "dates", Dates: []string{"2026-11-12", "2026-11-09", "2026-11-12", "2026-11-15"}}
	got, e := WorklogExportPeriods(in)
	if e != nil || len(got) != 3 || got[2].From != "2026-11-15" {
		t.Fatal(got, e)
	}
	in.Split = "none"
	got, e = WorklogExportPeriods(in)
	if e != nil || len(got) != 1 || got[0].To != in.To {
		t.Fatal(got, e)
	}
	got, e = WorklogExportPeriods(WorklogExportRequest{From: "9999-12-31", To: "9999-12-31"})
	if e != nil || !reflect.DeepEqual(got, []WorklogPeriod{{"9999-12-31", "9999-12-31"}}) {
		t.Fatal("last supported date", got, e)
	}
	for _, bad := range []WorklogExportRequest{
		{From: "2026-02-30", To: "2026-03-01"}, {From: "2026-02-10", To: "2026-02-09"}, {From: "2024-01-01", To: "2025-01-01"},
		{From: "2026-11-09", To: "2026-11-15", Split: "weekly"}, {From: "2026-11-09", To: "2026-11-15", Split: "weekly", Weekdays: []int{8}},
		{From: "2026-11-09", To: "2026-11-15", Split: "dates", Dates: []string{"2026-11-16"}},
		{From: "2026-11-09", To: "2026-11-15", Split: "wrong"}, {From: "2026-11-09", To: "2026-11-15", Boundary: "wrong"},
	} {
		if _, e := WorklogExportPeriods(bad); e == nil {
			t.Fatal("invalid request accepted", bad)
		}
	}
}

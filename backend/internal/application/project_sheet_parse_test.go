package application

import (
	"leadhelper/internal/domain"
	"testing"
)

func TestProjectSheetShapesAndStableRows(t *testing.T) {
	book := domain.ProjectSheetSnapshot{SpreadsheetID: "book", Title: "Projects", Sheets: []domain.ProjectSourceSheet{
		{ID: 1, Name: "Projects", Rows: [][]string{{"Клиент", "ИНН", "Статус 31.12.2025", "Статус 02.01"}, {"В работе"}, {"Alpha", "123", "Old", "New"}, {"Only name"}, {}, {"Закрытые вопросы"}, {"Beta", "", "Done"}}, BoldRows: []int{1, 5}, RowIDs: map[int]string{2: "stable"}},
		{ID: 2, Name: "Production", Hidden: true, Rows: [][]string{{"Дата", "Статус"}, {"01.01.2025", "One"}, {"", "Two"}, {"02.01.2025", "Three"}}},
		{ID: 3, Name: "Single", Rows: [][]string{{"Статус 01.01.2025", "Статус 02.01.2025"}, {"First", "Last"}}},
	}}
	out, e := ParseProjectSheet(book)
	if e != nil || len(out) != 5 {
		t.Fatalf("count %d %v", len(out), e)
	}
	if out[0].Status != "New" || len(out[0].History) != 2 || out[0].History[1].Date != "2026-01-02" || out[0].Fields[0].Value != "123" || out[1].Name != "Only name" || out[2].State != "completed" {
		t.Fatal(out)
	}
	if out[3].Status != "Three" || len(out[3].History) != 3 || out[3].History[1].Label != "01.01.2025" || out[4].Status != "Last" {
		t.Fatal(out)
	}
	id := out[0].ID
	book.Sheets[0].Rows = [][]string{book.Sheets[0].Rows[0], book.Sheets[0].Rows[2]}
	book.Sheets[0].BoldRows = nil
	book.Sheets[0].RowIDs = map[int]string{1: "stable"}
	out, e = ParseProjectSheet(book)
	if e != nil || out[0].ID != id {
		t.Fatal("row move changes identity")
	}
}
func TestProjectSheetDoesNotInventYear(t *testing.T) {
	if got := statusDates([]string{"01.01", "Статус 30.02.2026"}); got[1] != "" {
		t.Fatal(got)
	}
	if got := statusDates([]string{"01.01", "02.01"}); got[0] != "" || got[1] != "" {
		t.Fatal(got)
	}
}

func TestProjectSheetKeepsHiddenHistoryButMarksVisibility(t *testing.T) {
	book := domain.ProjectSheetSnapshot{SpreadsheetID: "book", Title: "Projects", VisibilityKnown: true, Sheets: []domain.ProjectSourceSheet{
		{ID: 1, Name: "Visible", Rows: [][]string{{"Name", "Статус"}, {"Heading"}, {"A", "Old"}, {"B", "New"}}, BoldRows: []int{1}, HiddenRows: []int{1, 2}},
		{ID: 2, Name: "Hidden", Hidden: true, Rows: [][]string{{"Дата", "Статус"}, {"01.11.2026", "Entry"}}},
	}}
	p, err := ParseProjectSheet(book)
	if err != nil || len(p) != 3 {
		t.Fatal(p, err)
	}
	if p[0].SourceVisibility != "row" || p[1].SourceVisibility != "" || p[2].SourceVisibility != "sheet" || len(p[0].History) != 1 || p[1].Group != "Visible / Heading" {
		t.Fatal("row flags, group, or history lost", p)
	}
}

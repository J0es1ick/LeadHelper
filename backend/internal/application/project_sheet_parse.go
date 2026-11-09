package application

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"leadhelper/internal/domain"
	"regexp"
	"strconv"
	"strings"
)

var statusDate = regexp.MustCompile(`(?i)(\d{1,2})[./](\d{1,2})(?:[./](\d{2,4}))?`)
var completedProjectStatus = regexp.MustCompile(`(?i)^(?:заверш[её]н[аоы]?|закрыт[аоы]?|выполнен[аоы]?|проект заверш[её]н|задача завершена|вопрос закрыт|done|closed|completed)[.!\s]*$`)

func sourceProjectID(book string, sheet int, key string) string {
	if strings.HasPrefix(key, "app:project-") {
		return strings.TrimPrefix(key, "app:")
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", book, sheet, key)))
	return "sheet-" + hex.EncodeToString(h[:12])
}
func sourceValue(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}
func statusLabel(s string) bool {
	return statusDate.MatchString(s) || strings.Contains(strings.ToLower(s), "статус") || strings.Contains(strings.ToLower(s), "статутс")
}
func statusDates(labels []string) []string {
	out := make([]string, len(labels))
	year := 0
	anchor := -1
	for i, s := range labels {
		m := statusDate.FindStringSubmatch(s)
		if len(m) > 3 && m[3] != "" {
			year, _ = strconv.Atoi(m[3])
			if year < 100 {
				year += 2000
			}
			anchor = i
			break
		}
	}
	if anchor < 0 {
		return out
	}
	prev := 0
	for i := anchor; i >= 0; i-- {
		m := statusDate.FindStringSubmatch(labels[i])
		if m == nil {
			continue
		}
		month, _ := strconv.Atoi(m[2])
		if prev > 0 && month > prev+6 {
			year--
		}
		prev = month
	}
	prev = 0
	for i, s := range labels {
		m := statusDate.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		d, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		if m[3] != "" {
			year, _ = strconv.Atoi(m[3])
			if year < 100 {
				year += 2000
			}
		} else if prev > 0 && month+6 < prev {
			year++
		}
		prev = month
		v := fmt.Sprintf("%04d-%02d-%02d", year, month, d)
		if validDate(v) {
			out[i] = v
		}
	}
	return out
}

// Parse the matrix losslessly: every non-empty status and non-status field is retained.
// Hidden rows and sheets retain their history but are excluded from the normal list.
// Bold single-cell rows are sections, not projects.
func ParseProjectSheet(book domain.ProjectSheetSnapshot) ([]domain.Project, error) {
	if book.SpreadsheetID == "" || len(book.Sheets) > 40 {
		return nil, invalid("Некорректная книга проектов")
	}
	out := []domain.Project{}
	cells := 0
	for _, s := range book.Sheets {
		if len(s.Rows) == 0 {
			continue
		}
		if len(s.Rows) > 5000 {
			return nil, invalid("В листе больше 5000 строк")
		}
		for _, row := range s.Rows {
			cells += len(row)
			if len(row) > 400 || cells > 300000 {
				return nil, invalid("Таблица превышает лимит 300 000 ячеек или 400 столбцов")
			}
		}
		headers := s.Rows[0]
		hiddenRows := map[int]bool{}
		for _, row := range s.HiddenRows {
			hiddenRows[row] = true
		}
		dates := statusDates(headers)
		group := ""
		closed := false
		vertical := strings.EqualFold(sourceValue(headers, 0), "Дата") && strings.EqualFold(sourceValue(headers, 1), "Статус")
		single := statusLabel(sourceValue(headers, 0))
		makeProject := func(name, key, mode string, row int) domain.Project {
			visibility := ""
			if s.Hidden {
				visibility = "sheet"
			} else if mode != "vertical" && hiddenRows[row] {
				visibility = "row"
			}
			if id := s.RowIDs[row]; id != "" {
				key = id
			}
			return domain.Project{SourceVisibility: visibility, ID: sourceProjectID(book.SpreadsheetID, s.ID, key), Name: name, State: "active", Group: s.Name, Source: book.Title, URL: fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit#gid=%d&range=A%d", book.SpreadsheetID, s.ID, row+1), Sheet: &domain.ProjectSheetRef{SpreadsheetID: book.SpreadsheetID, SheetID: s.ID, SheetName: s.Name, RowID: key, Row: row + 1, Mode: mode, Revision: s.Revisions[row]}, SyncState: "synced"}
		}
		if vertical {
			p := makeProject(strings.TrimSpace(s.Name), "sheet", "vertical", 0)
			labels := []string{}
			lastLabel := "Без даты"
			for _, r := range s.Rows[1:] {
				if v := sourceValue(r, 0); v != "" {
					lastLabel = v
				}
				labels = append(labels, lastLabel)
			}
			vd := statusDates(labels)
			for i, r := range s.Rows[1:] {
				if v := sourceValue(r, 1); v != "" {
					p.History = append(p.History, domain.ProjectStatus{Key: fmt.Sprintf("r%d", i+2), Label: labels[i], Date: vd[i], Text: v, Note: s.Notes[fmt.Sprintf("%d:1", i+1)], URL: s.Links[fmt.Sprintf("%d:1", i+1)]})
				}
			}
			finishSourceProject(&p)
			applyProjectOverrides(&p, s.Overrides[0])
			out = append(out, p)
			continue
		}
		for r, row := range s.Rows[1:] {
			if len(row) == 0 {
				continue
			}
			r++
			name := sourceValue(row, 0)
			if single {
				name = strings.TrimSpace(s.Name)
			}
			if name == "" {
				continue
			}
			hasOther := false
			for _, v := range row[1:] {
				hasOther = hasOther || strings.TrimSpace(v) != ""
			}
			bold := false
			for _, n := range s.BoldRows {
				bold = bold || n == r
			}
			if !single && !hasOther && bold {
				group = name
				closed = strings.Contains(strings.ToLower(name), "закрыт") || strings.Contains(strings.ToLower(name), "заверш")
				continue
			}
			p := makeProject(name, fmt.Sprintf("row:%d", r+1), "matrix", r)
			if single {
				p = makeProject(name, "sheet", "single", r)
			}
			if group != "" {
				p.Group = s.Name + " / " + group
			}
			if note, link := s.Notes[fmt.Sprintf("%d:0", r)], s.Links[fmt.Sprintf("%d:0", r)]; note != "" || link != "" {
				p.Fields = append(p.Fields, domain.ProjectField{Key: "name", Name: "Документ проекта", Value: link, URL: link, Note: note})
			}
			if closed {
				p.State = "completed"
			}
			first := 1
			if single {
				first = 0
			}
			for c := first; c < max(len(row), len(headers)); c++ {
				v := sourceValue(row, c)
				label := sourceValue(headers, c)
				if v == "" && (label == "" || statusLabel(label)) {
					continue
				}
				if label == "" {
					label = fmt.Sprintf("Столбец %d", c+1)
				}
				key := fmt.Sprintf("%d:%d", r, c)
				if statusLabel(label) {
					d := ""
					if c < len(dates) {
						d = dates[c]
					}
					p.History = append(p.History, domain.ProjectStatus{Key: fmt.Sprintf("c%d", c+1), Label: label, Date: d, Text: v, Note: s.Notes[key], URL: s.Links[key]})
				} else {
					p.Fields = append(p.Fields, domain.ProjectField{Key: fmt.Sprintf("c%d", c+1), Name: label, Value: v, Note: s.Notes[key], URL: s.Links[key]})
					if strings.Contains(strings.ToLower(label), "ответствен") {
						p.Owner = v
					}
				}
			}
			finishSourceProject(&p)
			applyProjectOverrides(&p, s.Overrides[r])
			out = append(out, p)
		}
	}
	return out, nil
}
func applyProjectOverrides(p *domain.Project, fields map[string]string) {
	for k, v := range fields {
		switch k {
		case "name":
			p.Name = v
		case "owner":
			p.Owner = v
		case "description":
			p.Description = v
		case "state":
			p.State = v
		case "startDate":
			p.StartDate = v
		case "dueDate":
			p.DueDate = v
		case "url":
			p.URL = v
		}
	}
}
func finishSourceProject(p *domain.Project) {
	p.Status = "Статус не указан"
	for _, h := range p.History {
		if v := strings.TrimSpace(h.Text); v != "" && v != "-" && v != "—" {
			p.Status = v
		}
	}
	p.HistoryCount = len(p.History)
	if completedProjectStatus.MatchString(p.Status) {
		p.State = "completed"
	}
}

package domain

import (
	"fmt"
	"sort"
	"time"
)

type WorklogExportRequest struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Split     string   `json:"split"`
	Boundary  string   `json:"boundary"`
	Weekdays  []int    `json:"weekdays"`
	Dates     []string `json:"dates"`
	MemberIDs []string `json:"memberIds"`
}
type WorklogPeriod struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type WorklogExportRow struct {
	MemberID  string  `json:"memberId"`
	Name      string  `json:"name"`
	Available bool    `json:"available"`
	Seconds   []int64 `json:"seconds"`
	Total     int64   `json:"total"`
}
type WorklogExportReport struct {
	From        string             `json:"from"`
	To          string             `json:"to"`
	GeneratedAt time.Time          `json:"generatedAt"`
	Periods     []WorklogPeriod    `json:"periods"`
	Rows        []WorklogExportRow `json:"rows"`
	Total       int64              `json:"total"`
	Missing     int                `json:"missing"`
	Records     int                `json:"records"`
}
type WorklogExportStatus struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Done      int    `json:"done"`
	Chunks    int    `json:"chunks"`
	Employees int    `json:"employees"`
	Missing   int    `json:"missing"`
	Error     string `json:"error,omitempty"`
}

// All boundaries are calendar dates in the workspace timezone. An end checkpoint
// includes that day; a start checkpoint belongs to the following period.
func WorklogExportPeriods(in WorklogExportRequest) ([]WorklogPeriod, error) {
	from, e := time.Parse("2006-01-02", in.From)
	if e != nil || from.Year() < 1900 {
		return nil, fmt.Errorf("Укажите корректную дату начала")
	}
	to, e := time.Parse("2006-01-02", in.To)
	if e != nil || to.Before(from) || to.Sub(from) > 365*24*time.Hour {
		return nil, fmt.Errorf("Выберите период от 1 до 366 дней")
	}
	if in.Boundary != "" && in.Boundary != "start" && in.Boundary != "end" {
		return nil, fmt.Errorf("Неизвестное правило контрольной даты")
	}
	bounds := map[time.Time]bool{from: true, to.AddDate(0, 0, 1): true}
	add := func(t time.Time) {
		if in.Boundary == "end" {
			t = t.AddDate(0, 0, 1)
		}
		if t.After(from) && !t.After(to) {
			bounds[t] = true
		}
	}
	switch in.Split {
	case "", "none":
	case "weekly":
		if len(in.Weekdays) == 0 || len(in.Weekdays) > 7 {
			return nil, fmt.Errorf("Выберите дни недели для контрольных точек")
		}
		days := map[int]bool{}
		for _, day := range in.Weekdays {
			if day < 1 || day > 7 {
				return nil, fmt.Errorf("Некорректный день недели")
			}
			days[day] = true
		}
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			if days[(int(d.Weekday())+6)%7+1] {
				add(d)
			}
		}
	case "dates":
		if len(in.Dates) == 0 || len(in.Dates) > 366 {
			return nil, fmt.Errorf("Добавьте от 1 до 366 контрольных дат")
		}
		for _, value := range in.Dates {
			d, e := time.Parse("2006-01-02", value)
			if e != nil || d.Before(from) || d.After(to) {
				return nil, fmt.Errorf("Контрольные даты должны находиться внутри общего периода")
			}
			add(d)
		}
	default:
		return nil, fmt.Errorf("Неизвестный способ разбиения периода")
	}
	keys := make([]time.Time, 0, len(bounds))
	for key := range bounds {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })
	out := make([]WorklogPeriod, 0, len(keys)-1)
	for i := 0; i < len(keys)-1; i++ {
		out = append(out, WorklogPeriod{keys[i].Format("2006-01-02"), keys[i+1].AddDate(0, 0, -1).Format("2006-01-02")})
	}
	return out, nil
}

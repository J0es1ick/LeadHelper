package application

import (
	"context"
	"leadhelper/internal/domain"
	"time"
)

func (s *Service) Report(ctx context.Context, from, to, source string) (domain.Report, error) {
	r := domain.Report{From: from, To: to, Tasks: []domain.Task{}, Workload: []domain.Workload{}}
	if from == "" || to == "" || !validDate(from) || !validDate(to) || from > to {
		return r, invalid("Укажите корректный диапазон дат")
	}
	st, e := s.State(ctx)
	if e != nil {
		return r, e
	}
	if e = enabled(&st, "reports"); e != nil {
		return r, e
	}
	today := time.Now().In(time.FixedZone("MSK", 3*3600)).Format("2006-01-02")
	planned := map[string]bool{}
	for _, entry := range st.TeamPlan {
		if entry.Demo == st.Settings.Demo && entry.Date >= from && entry.Date <= to {
			planned[entry.TaskID] = true
		}
	}
	for _, t := range st.Tasks {
		if t.Personal || (source != "all" && source != "" && source != t.Source) {
			continue
		}
		date := t.PlannedDate
		if st.TeamPlanVersion >= 1 {
			date = ""
		}
		if date == "" {
			date = t.DueDate
		}
		completed := t.CompletedAt != nil && t.CompletedAt.Format("2006-01-02") >= from && t.CompletedAt.Format("2006-01-02") <= to
		if (date < from || date > to) && !completed && !planned[t.ID] {
			continue
		}
		r.Tasks = append(r.Tasks, t)
		r.Total++
		if completed {
			r.Completed++
		}
		if t.Status == "progress" || t.Status == "review" {
			r.InProgress++
		}
		if t.Status != "done" && t.DueDate != "" && t.DueDate < today {
			r.Overdue++
		}
		r.PlannedHours += t.Estimate
	}
	start, _ := time.Parse("2006-01-02", from)
	end, _ := time.Parse("2006-01-02", to)
	days := 0
	if end.Sub(start) > 366*24*time.Hour {
		return r, invalid("Максимальный период — один год")
	}
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			days++
		}
	}
	if st.Settings.Modules["team"] {
		for _, m := range st.Members {
			if m.Inactive {
				continue
			}
			w := domain.Workload{Member: m}
			if st.TeamPlanVersion >= 1 {
				visible := map[string]bool{}
				for _, task := range r.Tasks {
					visible[task.ID] = true
				}
				unique := map[string]bool{}
				for _, entry := range st.TeamPlan {
					if entry.Demo == st.Settings.Demo && entry.MemberID == m.ID && entry.Date >= from && entry.Date <= to && visible[entry.TaskID] {
						w.Hours += entry.Hours
						unique[entry.TaskID] = true
					}
				}
				w.Tasks = len(unique)
			} else {
				for _, t := range r.Tasks {
					if t.AssigneeID == m.ID && t.Status != "done" {
						w.Hours += t.Estimate
						w.Tasks++
					}
				}
			}
			capacity := m.Capacity * float64(days) / 5
			if capacity > 0 {
				w.Percent = w.Hours / capacity * 100
			}
			r.Workload = append(r.Workload, w)
		}
	}
	return r, nil
}

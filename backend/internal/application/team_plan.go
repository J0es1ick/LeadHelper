package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"math"
	"slices"
	"sort"
	"time"
)

func (s *Service) UpgradeTeamPlans(ctx context.Context) error {
	st, e := s.taskShell(ctx)
	if e != nil || st.TeamPlanVersion >= 1 {
		return e
	}
	return s.updateCore(ctx, func(current *domain.State) error {
		if current.TeamPlanVersion >= 1 {
			return nil
		}
		for _, t := range current.Tasks {
			if !t.Personal && t.PlannedDate != "" && validDate(t.PlannedDate) && t.AssigneeID != "" {
				current.TeamPlan = append(current.TeamPlan, domain.TeamPlanEntry{MemberID: t.AssigneeID, Date: t.PlannedDate, TaskID: t.ID, Hours: t.Estimate, Demo: t.Demo})
			}
		}
		current.TeamPlanVersion = 1
		return nil
	})
}
func (s *Service) planEntries(ctx context.Context, demo bool, from, to, member string) ([]domain.TeamPlanEntry, error) {
	if r, ok := s.repo.(ports.PagedTaskRepository); ok {
		return r.PlanEntries(ctx, demo, from, to, member)
	}
	st, e := s.viewCore(ctx)
	out := []domain.TeamPlanEntry{}
	for _, entry := range st.TeamPlan {
		if entry.Demo == demo && entry.Date >= from && entry.Date <= to && (member == "" || entry.MemberID == member) {
			out = append(out, entry)
		}
	}
	return out, e
}
func validPlanRange(from, to string) bool {
	a, e := time.Parse("2006-01-02", from)
	b, f := time.Parse("2006-01-02", to)
	return e == nil && f == nil && !b.Before(a) && b.Sub(a) <= 31*24*time.Hour
}
func (s *Service) TeamWeek(ctx context.Context, from, to string) (domain.TeamWeekPlan, error) {
	out := domain.TeamWeekPlan{Cells: []domain.TeamPlanCell{}}
	st, e := s.taskShell(ctx)
	if e != nil {
		return out, e
	}
	if e = enabled(&st, "team"); e != nil {
		return out, e
	}
	if !validPlanRange(from, to) {
		return out, invalid("Выберите период до 31 дня")
	}
	entries, e := s.planEntries(ctx, st.Settings.Demo, from, to, "")
	if e != nil {
		return out, e
	}
	cells := map[string]*domain.TeamPlanCell{}
	cache := map[string]domain.Task{}
	for _, entry := range entries {
		t, ok := cache[entry.TaskID]
		if !ok {
			t, e = s.Task(ctx, entry.TaskID)
			if errors.Is(e, ErrNotFound) {
				continue
			}
			if e != nil {
				return out, e
			}
			cache[entry.TaskID] = t
		}
		if t.Personal {
			continue
		}
		key := entry.MemberID + "/" + entry.Date
		cell := cells[key]
		if cell == nil {
			cell = &domain.TeamPlanCell{MemberID: entry.MemberID, Date: entry.Date, Tasks: []domain.Task{}}
			cells[key] = cell
		}
		cell.Count++
		cell.Hours += entry.Hours
		if entry.Hours == 0 {
			cell.Unestimated++
		}
		if len(cell.Tasks) < 3 {
			cell.Tasks = append(cell.Tasks, taskSummary(t))
		}
	}
	for _, cell := range cells {
		out.Cells = append(out.Cells, *cell)
	}
	sort.Slice(out.Cells, func(i, j int) bool {
		a, b := out.Cells[i], out.Cells[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		return a.MemberID < b.MemberID
	})
	return out, nil
}
func dayRevision(entries []domain.TeamPlanEntry) string {
	sorted := append([]domain.TeamPlanEntry{}, entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TaskID < sorted[j].TaskID })
	raw, _ := json.Marshal(sorted)
	hash := sha256.Sum256(raw)
	return fmt.Sprintf("%x", hash[:16])
}
func planMember(st domain.State, id string) bool {
	return slices.ContainsFunc(st.Members, func(m domain.Member) bool { return m.ID == id && m.Demo == st.Settings.Demo && !m.Inactive })
}
func (s *Service) TeamDay(ctx context.Context, member, date string, q domain.TaskQuery) (domain.TeamDayPlan, error) {
	out := domain.TeamDayPlan{Selected: []domain.PlannedTask{}}
	st, e := s.taskShell(ctx)
	if e != nil {
		return out, e
	}
	if e = enabled(&st, "team"); e != nil {
		return out, e
	}
	if !validPlanRange(date, date) || !planMember(st, member) {
		return out, invalid("Неверная дата или сотрудник больше не входит в команду")
	}
	q.Assignee = member
	q.Personal = "false"
	q.Status = "actionable"
	q.From = ""
	q.To = ""
	out.Candidates, e = s.TaskPage(ctx, q)
	if e != nil {
		return out, e
	}
	entries, e := s.planEntries(ctx, st.Settings.Demo, date, date, member)
	if e != nil {
		return out, e
	}
	out.Revision = dayRevision(entries)
	for _, entry := range entries {
		t, err := s.Task(ctx, entry.TaskID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return out, err
		}
		if !t.Personal {
			out.Selected = append(out.Selected, domain.PlannedTask{TeamPlanEntry: entry, Task: taskSummary(t)})
		}
	}
	return out, nil
}
func (s *Service) SaveTeamDay(ctx context.Context, member, date string, input domain.TeamDayInput) error {
	if !validPlanRange(date, date) || len(input.Entries) > 200 {
		return invalid("Некорректная дата или больше 200 задач на день")
	}
	return s.updateCore(ctx, func(st *domain.State) error {
		if e := enabled(st, "team"); e != nil {
			return e
		}
		if e := enabled(st, "backlog"); e != nil {
			return e
		}
		if !planMember(*st, member) {
			return invalid("Сотрудник больше не входит в команду")
		}
		old := []domain.TeamPlanEntry{}
		existing := map[string]bool{}
		tasks := map[string]domain.Task{}
		for _, t := range st.Tasks {
			if t.Demo == st.Settings.Demo {
				tasks[t.ID] = t
			}
		}
		for _, entry := range st.TeamPlan {
			if entry.Demo == st.Settings.Demo && entry.MemberID == member && entry.Date == date {
				old = append(old, entry)
				existing[entry.TaskID] = true
			}
		}
		if input.Revision != dayRevision(old) {
			return ErrConflict
		}
		next := []domain.TeamPlanEntry{}
		seen := map[string]bool{}
		for _, entry := range input.Entries {
			t, ok := tasks[entry.TaskID]
			if !ok || !taskVisible(*st, t) || t.Personal {
				return invalid("Одна из задач больше недоступна. Обновите список")
			}
			if !existing[t.ID] && (t.AssigneeID != member || (t.Status != "todo" && t.Status != "progress")) {
				return invalid("Новые задачи должны быть назначены этому сотруднику и находиться в статусе «К выполнению» или «В работе»")
			}
			if seen[t.ID] || math.IsNaN(entry.Hours) || math.IsInf(entry.Hours, 0) || entry.Hours < 0 || entry.Hours > 24 {
				return invalid("Повторная задача или некорректные часы (от 0 до 24)")
			}
			seen[t.ID] = true
			next = append(next, domain.TeamPlanEntry{MemberID: member, Date: date, TaskID: t.ID, Hours: entry.Hours, Demo: st.Settings.Demo})
		}
		st.TeamPlan = slices.DeleteFunc(st.TeamPlan, func(entry domain.TeamPlanEntry) bool {
			if entry.Demo != st.Settings.Demo || entry.MemberID != member || entry.Date != date {
				return false
			}
			if t, ok := tasks[entry.TaskID]; ok && !taskVisible(*st, t) {
				return false
			}
			return true
		})
		st.TeamPlan = append(st.TeamPlan, next...)
		st.TeamPlanVersion = 1
		return nil
	})
}

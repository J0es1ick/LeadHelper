package application

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"slices"
	"sort"
	"strings"
	"time"
)

func (s *Service) taskShell(ctx context.Context) (domain.State, error) {
	if r, ok := s.repo.(ports.PagedTaskRepository); ok {
		return r.ViewShell(ctx)
	}
	return s.viewCore(ctx)
}
func taskVisible(st domain.State, t domain.Task) bool {
	return t.Demo == st.Settings.Demo && t.Source != "bitrix" && st.Settings.Modules[taskModule(t)] && (t.Source == "local" || st.Settings.Modules[t.Source])
}
func taskSummary(t domain.Task) domain.Task { t.Description = ""; return t }
func normalizeTaskQuery(st domain.State, q domain.TaskQuery) (domain.TaskQuery, error) {
	if len(q.Search) > 200 || len(q.Assignee) > 250 {
		return q, invalid("Слишком длинный фильтр задач")
	}
	q.Search = strings.TrimSpace(q.Search)
	q.Demo = st.Settings.Demo
	q.Limit = min(50, max(1, q.Limit))
	q.Page = max(1, q.Page)
	if !slices.Contains([]string{"", "all", "active", "actionable", "backlog", "todo", "progress", "review", "done"}, q.Status) {
		return q, invalid("Неизвестный статус задач")
	}
	if !slices.Contains([]string{"false", "true", "all"}, q.Personal) {
		return q, invalid("Неизвестный тип задач")
	}
	if q.From != "" && q.From != "none" && (!validDate(q.From) || !validDate(q.To) || q.To == "" || q.From > q.To) {
		return q, invalid("Некорректный период задач")
	}
	if q.Personal == "all" {
		if !st.Settings.Modules["planner"] {
			q.Personal = "false"
		} else if !st.Settings.Modules["backlog"] {
			q.Personal = "true"
		}
	}
	if q.Personal == "true" && !st.Settings.Modules["planner"] || q.Personal == "false" && !st.Settings.Modules["backlog"] {
		return q, ErrDisabled
	}
	q.Sources = []string{"local"}
	if st.Settings.Modules["jira"] {
		q.Sources = append(q.Sources, "jira")
	}
	return q, nil
}
func matchesTask(t domain.Task, q domain.TaskQuery) bool {
	if t.Demo != q.Demo || !slices.Contains(q.Sources, t.Source) || q.Personal != "all" && t.Personal != (q.Personal == "true") {
		return false
	}
	if q.Source != "" && q.Source != "all" && t.Source != q.Source || q.Assignee != "all" && t.AssigneeID != q.Assignee || q.Priority != "" && q.Priority != "all" && t.Priority != q.Priority {
		return false
	}
	if q.Status == "active" && t.Status == "done" || q.Status == "actionable" && t.Status != "todo" && t.Status != "progress" {
		return false
	}
	if q.Status != "" && q.Status != "all" && q.Status != "active" && q.Status != "actionable" && t.Status != q.Status {
		return false
	}
	if q.From == "none" && t.PlannedDate != "" || q.From != "" && q.From != "none" && (t.PlannedDate < q.From || t.PlannedDate > q.To) {
		return false
	}
	return q.Search == "" || strings.Contains(strings.ToLower(t.Title+" "+t.Description+" "+t.ExternalID+" "+t.Project), strings.ToLower(q.Search))
}
func (s *Service) TaskPage(ctx context.Context, q domain.TaskQuery) (domain.TaskPage, error) {
	st, e := s.taskShell(ctx)
	if e != nil {
		return domain.TaskPage{}, e
	}
	q, e = normalizeTaskQuery(st, q)
	if e != nil {
		return domain.TaskPage{}, e
	}
	if r, ok := s.repo.(ports.PagedTaskRepository); ok {
		return r.TaskPage(ctx, q)
	}
	items := []domain.Task{}
	for _, t := range st.Tasks {
		if matchesTask(t, q) {
			items = append(items, t)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if q.Sort == "due" {
			ad, bd := a.DueDate, b.DueDate
			if ad == "" {
				ad = "9999"
			}
			if bd == "" {
				bd = "9999"
			}
			if ad != bd {
				return ad < bd
			}
		} else if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.ID < b.ID
	})
	out := domain.TaskPage{Items: []domain.Task{}, Total: len(items), Limit: q.Limit, Pages: max(1, (len(items)+q.Limit-1)/q.Limit)}
	out.Page = min(q.Page, out.Pages)
	for _, t := range items[(out.Page-1)*q.Limit : min(out.Page*q.Limit, len(items))] {
		out.Items = append(out.Items, taskSummary(t))
	}
	return out, nil
}
func (s *Service) Task(ctx context.Context, id string) (domain.Task, error) {
	st, e := s.taskShell(ctx)
	if e != nil {
		return domain.Task{}, e
	}
	if r, ok := s.repo.(ports.PagedTaskRepository); ok {
		t, e := r.Task(ctx, id, st.Settings.Demo)
		if errors.Is(e, sql.ErrNoRows) {
			e = ErrNotFound
		}
		if e == nil && !taskVisible(st, t) {
			e = ErrNotFound
		}
		return t, e
	}
	for _, t := range st.Tasks {
		if t.ID == id && taskVisible(st, t) {
			return t, nil
		}
	}
	return domain.Task{}, ErrNotFound
}
func (s *Service) TaskStatus(ctx context.Context, id, status string) error {
	if !slices.Contains([]string{"backlog", "todo", "progress", "review", "done"}, status) {
		return invalid("Неизвестный статус")
	}
	return s.updateCore(ctx, func(st *domain.State) error {
		for i := range st.Tasks {
			t := &st.Tasks[i]
			if t.ID != id || !taskVisible(*st, *t) {
				continue
			}
			t.Status = status
			t.UpdatedAt = time.Now().UTC()
			if status == "done" {
				if t.CompletedAt == nil {
					t.CompletedAt = &t.UpdatedAt
				}
			} else {
				t.CompletedAt = nil
			}
			return nil
		}
		return ErrNotFound
	})
}
func moscowWeek() (string, string, string) {
	now := time.Now().In(time.FixedZone("MSK", 10800))
	start := now.AddDate(0, 0, -(int(now.Weekday())+6)%7)
	return now.Format("2006-01-02"), start.Format("2006-01-02"), start.AddDate(0, 0, 6).Format("2006-01-02")
}
func (s *Service) WorkspaceState(ctx context.Context) (domain.State, error) {
	st, e := s.taskShell(ctx)
	if e != nil {
		return st, e
	}
	all := st.Tasks
	if st.Revision == "" {
		raw, _ := json.Marshal(struct {
			Tasks    []domain.Task
			Plan     []domain.TeamPlanEntry
			Settings domain.Settings
		}{all, st.TeamPlan, st.Settings})
		sum := sha256.Sum256(raw)
		st.Revision = fmt.Sprintf("%x", sum[:12])
	}
	st.Tasks = []domain.Task{}
	st.TeamPlan = nil
	st, e = s.visibleState(ctx, st)
	if e != nil {
		return st, e
	}
	today, from, to := moscowWeek()
	q := domain.TaskQuery{Demo: st.Settings.Demo, Personal: "all", Assignee: "all", Sources: []string{"local"}}
	if st.Settings.Modules["jira"] {
		q.Sources = append(q.Sources, "jira")
	}
	metrics := domain.TaskMetrics{Statuses: map[string]int{}, MemberTotals: map[string]int{}, MemberActive: map[string]int{}, MemberHours: map[string]float64{}}
	if r, ok := s.repo.(ports.PagedTaskRepository); ok {
		metrics, e = r.TaskMetrics(ctx, q, today)
		if e != nil {
			return st, e
		}
	} else {
		for _, t := range all {
			if !taskVisible(st, t) {
				continue
			}
			if t.Personal {
				if t.PlannedDate == today {
					metrics.PersonalTotal++
					if t.Status == "done" {
						metrics.PersonalDone++
					}
				}
				continue
			}
			metrics.Statuses[t.Status]++
			metrics.MemberTotals[t.AssigneeID]++
			if t.Status != "done" {
				metrics.MemberActive[t.AssigneeID]++
				if t.DueDate != "" && t.DueDate < today {
					metrics.Overdue++
				}
			}
		}
	}
	if st.Settings.Modules["backlog"] {
		p, err := s.TaskPage(ctx, domain.TaskQuery{Personal: "false", Assignee: "all", Status: "active", Sort: "due", Limit: 5, Page: 1})
		if err != nil {
			return st, err
		}
		st.Tasks = append(st.Tasks, p.Items...)
	} else {
		metrics.Statuses = map[string]int{}
		metrics.MemberTotals = map[string]int{}
		metrics.MemberActive = map[string]int{}
		metrics.Overdue = 0
	}
	if st.Settings.Modules["planner"] {
		p, err := s.TaskPage(ctx, domain.TaskQuery{Personal: "true", Assignee: "all", From: today, To: today, Limit: 25, Page: 1})
		if err != nil {
			return st, err
		}
		st.Tasks = append(st.Tasks, p.Items...)
	} else {
		metrics.PersonalTotal = 0
		metrics.PersonalDone = 0
	}
	if st.Settings.Modules["team"] {
		plan, err := s.TeamWeek(ctx, from, to)
		if err != nil {
			return st, err
		}
		for _, cell := range plan.Cells {
			metrics.MemberHours[cell.MemberID] += cell.Hours
		}
		if !st.Settings.Demo && st.Settings.Modules["jira"] {
			if source, ok := s.sources["jira"].(ports.JiraWorklogSource); ok && source != nil && s.sources["jira"].Configured() {
				metrics.WorklogAvailable = true
				if actual, err := s.JiraWorklogWeek(ctx, from, to); err == nil {
					metrics.WorklogHours = map[string]float64{}
					metrics.WorklogSyncedAt = actual.SyncedAt
					for _, cell := range actual.Cells {
						metrics.WorklogHours[cell.MemberID] += float64(cell.Seconds) / 3600
					}
				}
			}
		}
	}
	st.TaskMetrics = &metrics
	return st, nil
}

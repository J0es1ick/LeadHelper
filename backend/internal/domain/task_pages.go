package domain

import "time"

type TaskQuery struct {
	Demo                                                       bool
	Sources                                                    []string
	Personal                                                   string
	Status, Source, Assignee, Priority, Search, Sort, From, To string
	Page, Limit                                                int
}
type TaskPage struct {
	Items []Task `json:"items"`
	Total int    `json:"total"`
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
	Pages int    `json:"pages"`
}
type TaskMetrics struct {
	WorklogHours     map[string]float64 `json:"worklogHours,omitempty"`
	WorklogSyncedAt  *time.Time         `json:"worklogSyncedAt,omitempty"`
	WorklogAvailable bool               `json:"worklogAvailable"`
	Statuses         map[string]int     `json:"statuses"`
	Overdue          int                `json:"overdue"`
	PersonalTotal    int                `json:"personalTotal"`
	PersonalDone     int                `json:"personalDone"`
	MemberTotals     map[string]int     `json:"memberTotals"`
	MemberActive     map[string]int     `json:"memberActive"`
	MemberHours      map[string]float64 `json:"memberHours"`
}
type TeamPlanEntry struct {
	MemberID string  `json:"memberId"`
	Date     string  `json:"date"`
	TaskID   string  `json:"taskId"`
	Hours    float64 `json:"hours"`
	Demo     bool    `json:"demo"`
}
type PlannedTask struct {
	TeamPlanEntry
	Task Task `json:"task"`
}
type TeamPlanCell struct {
	MemberID    string  `json:"memberId"`
	Date        string  `json:"date"`
	Count       int     `json:"count"`
	Hours       float64 `json:"hours"`
	Unestimated int     `json:"unestimated"`
	Tasks       []Task  `json:"tasks"`
}
type TeamWeekPlan struct {
	Cells []TeamPlanCell `json:"cells"`
}
type TeamDayPlan struct {
	Candidates TaskPage      `json:"candidates"`
	Selected   []PlannedTask `json:"selected"`
	Revision   string        `json:"revision"`
}
type TeamDayInput struct {
	Revision string          `json:"revision"`
	Entries  []TeamPlanEntry `json:"entries"`
}

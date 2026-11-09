package domain

import "time"

type TeamDepartment struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty"`
}
type TeamPerson struct {
	ID          string
	Email       string
	Name        string
	Role        string
	Departments []TeamDepartment
}
type TeamSnapshot struct {
	AccountID   string
	ManagerID   string
	ManagerName string
	Roots       []TeamDepartment
	Departments []TeamDepartment
	Members     []TeamPerson
}
type TeamSyncStatus struct {
	Enabled         bool             `json:"enabled"`
	AccountID       string           `json:"accountId,omitempty"`
	ManagerName     string           `json:"managerName,omitempty"`
	Roots           []TeamDepartment `json:"roots,omitempty"`
	DepartmentCount int              `json:"departmentCount"`
	MemberCount     int              `json:"memberCount"`
	LastAttemptAt   *time.Time       `json:"lastAttemptAt,omitempty"`
	LastSuccessAt   *time.Time       `json:"lastSuccessAt,omitempty"`
	Error           string           `json:"error,omitempty"`
}

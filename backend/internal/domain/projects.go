package domain

import "time"

type Project struct {
	SourceVisibility string           `json:"sourceVisibility,omitempty"`
	RemoteVersion    *Project         `json:"remoteVersion,omitempty"`
	Group            string           `json:"group,omitempty"`
	History          []ProjectStatus  `json:"history,omitempty"`
	HistoryCount     int              `json:"historyCount,omitempty"`
	Fields           []ProjectField   `json:"fields,omitempty"`
	Sheet            *ProjectSheetRef `json:"sheet,omitempty"`
	SyncState        string           `json:"syncState,omitempty"`
	SyncMessage      string           `json:"syncMessage,omitempty"`
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Status           string           `json:"status"`
	State            string           `json:"state"`
	Owner            string           `json:"owner"`
	Description      string           `json:"description"`
	StartDate        string           `json:"startDate"`
	DueDate          string           `json:"dueDate"`
	URL              string           `json:"url"`
	Source           string           `json:"source"`
	Version          int              `json:"version"`
	UpdatedAt        time.Time        `json:"updatedAt"`
}
type ProjectPage struct {
	HiddenTotal int            `json:"hiddenTotal"`
	Groups      []string       `json:"groups"`
	Items       []Project      `json:"items"`
	Total       int            `json:"total"`
	Page        int            `json:"page"`
	Limit       int            `json:"limit"`
	Counts      map[string]int `json:"counts"`
}
type ProjectQuery struct {
	IncludeHidden        bool
	Search, State, Group string
	Folder               string
	Unfiled              bool
	Page, Limit          int
}

type ProjectImportBatch struct {
	Projects []Project `json:"projects"`
	Fields   []string  `json:"fields"`
}

type ProjectStatus struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Date  string `json:"date,omitempty"`
	Text  string `json:"text"`
	Note  string `json:"note,omitempty"`
	URL   string `json:"url,omitempty"`
}
type ProjectField struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Value string `json:"value"`
	Note  string `json:"note,omitempty"`
	URL   string `json:"url,omitempty"`
}
type ProjectSheetRef struct {
	Revision      string `json:"revision,omitempty"`
	SpreadsheetID string `json:"spreadsheetId"`
	SheetID       int    `json:"sheetId"`
	SheetName     string `json:"sheetName"`
	RowID         string `json:"rowId"`
	Row           int    `json:"row"`
	Mode          string `json:"mode"`
}
type ProjectSheetSnapshot struct {
	VisibilityKnown bool                 `json:"visibilityKnown,omitempty"`
	SpreadsheetID   string               `json:"spreadsheetId"`
	Title           string               `json:"title"`
	FetchedAt       time.Time            `json:"fetchedAt"`
	Sheets          []ProjectSourceSheet `json:"sheets"`
}
type ProjectSourceSheet struct {
	HiddenRows []int                     `json:"hiddenRows,omitempty"`
	Revisions  map[int]string            `json:"revisions,omitempty"`
	Overrides  map[int]map[string]string `json:"overrides,omitempty"`
	ID         int                       `json:"id"`
	Name       string                    `json:"name"`
	Rows       [][]string                `json:"rows"`
	BoldRows   []int                     `json:"boldRows,omitempty"`
	RowIDs     map[int]string            `json:"rowIds,omitempty"`
	Notes      map[string]string         `json:"notes,omitempty"`
	Links      map[string]string         `json:"links,omitempty"`
	Hidden     bool                      `json:"hidden,omitempty"`
}
type ProjectSheetChange struct {
	Project     Project `json:"project"`
	Base        Project `json:"base"`
	OperationID string  `json:"operationId"`
}
type ProjectSyncStatus struct {
	VisibilityKnown bool      `json:"visibilityKnown,omitempty"`
	Configured      bool      `json:"configured"`
	Title           string    `json:"title"`
	SpreadsheetID   string    `json:"spreadsheetId"`
	UpdatedAt       time.Time `json:"updatedAt"`
	Error           string    `json:"error,omitempty"`
	Pending         int       `json:"pending"`
	Conflicts       int       `json:"conflicts"`
}

package application

import (
	"context"
	"database/sql"
	"errors"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"net/url"
	"slices"
	"strings"
)

func (s *Service) projectContext(ctx context.Context) (ports.ProjectRepository, bool, error) {
	st, e := s.taskShell(ctx)
	if e != nil {
		return nil, false, e
	}
	if e = enabled(&st, "projects"); e != nil {
		return nil, false, e
	}
	r, ok := s.repo.(ports.ProjectRepository)
	if !ok {
		return nil, false, invalid("Хранилище проектов недоступно")
	}
	return r, st.Settings.Demo, nil
}
func (s *Service) Projects(ctx context.Context, q domain.ProjectQuery) (domain.ProjectPage, error) {
	r, demo, e := s.projectContext(ctx)
	if e != nil {
		return domain.ProjectPage{}, e
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit == 0 {
		q.Limit = 25
	}
	if q.State == "" {
		q.State = "active"
	}
	q.Search = strings.TrimSpace(q.Search)
	if len(q.Group) > 1000 || len(q.Folder) > 1000 {
		return domain.ProjectPage{}, invalid("Слишком длинное название папки")
	}
	if q.Limit < 1 || q.Limit > 100 || q.Page > 100000 || len(q.Search) > 300 || !slices.Contains([]string{"active", "completed", "archived", "all"}, q.State) {
		return domain.ProjectPage{}, invalid("Некорректный фильтр проектов")
	}
	return r.Projects(ctx, demo, q)
}
func cleanProject(p *domain.Project) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Status = strings.TrimSpace(p.Status)
	p.Owner = strings.TrimSpace(p.Owner)
	p.Group = strings.TrimSpace(p.Group)
	if len(p.Group) > 1000 || len(p.History) > 400 {
		return invalid("Слишком длинное название папки или более 400 записей истории проекта")
	}
	keys := map[string]bool{}
	for _, h := range p.History {
		if h.Key == "" || keys[h.Key] || len(h.Key) > 100 || len(h.Label) > 1000 || len(h.Text) > 60000 || len(h.Note) > 60000 || len(h.URL) > 2000 || !validDate(h.Date) {
			return invalid("Некорректная история статусов проекта")
		}
		keys[h.Key] = true
	}
	if p.Status == "" {
		p.Status = "В работе"
	}
	if p.State == "" {
		p.State = "active"
	}
	if p.Name == "" || len(p.Name) > 1000 || len(p.Status) > 60000 || len(p.Owner) > 1000 || len(p.Description) > 20000 || len(p.URL) > 2000 || len(p.Source) > 300 || len(p.ID) > 120 || p.Version < 0 {
		return invalid("Проверьте название и длину полей проекта")
	}
	if !slices.Contains([]string{"active", "completed", "archived"}, p.State) || !validDate(p.StartDate) || !validDate(p.DueDate) || (p.StartDate != "" && p.DueDate != "" && p.DueDate < p.StartDate) {
		return invalid("Проверьте состояние и даты проекта")
	}
	if p.URL != "" {
		u, e := url.Parse(p.URL)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			return invalid("Ссылка проекта должна начинаться с https:// или http://")
		}
	}
	return nil
}
func (s *Service) SaveProject(ctx context.Context, p domain.Project) error {
	p.SourceVisibility = ""
	r, demo, e := s.projectContext(ctx)
	if e != nil {
		return e
	}
	if e = cleanProject(&p); e != nil {
		return e
	}
	if p.ID == "" {
		p.ID = ID("project-")
		p.Version = 0
		if p.SyncState == "new-sheet" && !demo {
			if source, ok := r.(ports.ProjectSyncRepository); ok {
				status, e := source.ProjectSyncStatus(ctx)
				if e != nil {
					return e
				}
				if status.SpreadsheetID == "" {
					return invalid("Сначала подключите таблицу проектов")
				}
				p.Sheet = &domain.ProjectSheetRef{SpreadsheetID: status.SpreadsheetID, SheetID: -1, SheetName: "Проекты LeadHelper", RowID: "app:" + p.ID, Mode: "new"}
				p.Source = status.Title
			}
		} else {
			p.Sheet = nil
			p.SyncState = ""
		}
	}
	return projectError(r.SaveProjects(ctx, demo, []domain.Project{p}, ""))
}
func projectError(e error) error {
	if e != nil {
		return invalid(e.Error())
	}
	return nil
}
func (s *Service) PreviewProjects(ctx context.Context, rows []domain.Project, merge bool, fields []string) ([]domain.Project, error) {
	return s.PreviewProjectBatches(ctx, []domain.ProjectImportBatch{{Projects: rows, Fields: fields}}, merge)
}
func (s *Service) PreviewProjectBatches(ctx context.Context, batches []domain.ProjectImportBatch, merge bool) ([]domain.Project, error) {
	r, demo, e := s.projectContext(ctx)
	if e != nil {
		return nil, e
	}
	total := 0
	for _, batch := range batches {
		total += len(batch.Projects)
	}
	if total == 0 || total > 2000 || len(batches) > 40 {
		return nil, invalid("Импортируйте от 1 до 2000 проектов и до 40 листов за один раз")
	}
	seen := map[string]bool{}
	result := make([]domain.Project, 0, total)
	for _, batch := range batches {
		rows, fields := batch.Projects, batch.Fields
		for i := range rows {
			p := &rows[i]
			p.SourceVisibility = ""
			if e = cleanProject(p); e != nil {
				return nil, e
			}
			key := p.Group + "\x00" + strings.ToLower(p.Name)
			if seen[key] {
				return nil, invalid("Повторяющееся название в папке «" + p.Group + "»: " + p.Name)
			}
			seen[key] = true
			p.ID = ID("project-")
			p.Version = 0
			p.UpdatedAt = domain.Project{}.UpdatedAt
			p.Sheet, p.RemoteVersion, p.Fields = nil, nil, nil
			p.SyncState, p.SyncMessage = "", ""
			if merge {
				var old domain.Project
				var e error
				if slices.Contains(fields, "group") {
					old, e = r.ProjectByNameInGroup(ctx, demo, p.Name, p.Group)
				} else {
					old, e = r.ProjectByName(ctx, demo, p.Name)
				}
				if e == nil {
					if old.Sheet != nil {
						return nil, invalid("Проект «" + p.Name + "» связан с Google Таблицей. Используйте синхронизацию или выберите другую папку для импорта")
					}
					p.ID = old.ID
					p.Version = old.Version
					if !slices.Contains(fields, "group") {
						p.Group = old.Group
					}
					if !slices.Contains(fields, "history") {
						p.History = old.History
					}
					if !slices.Contains(fields, "status") {
						p.Status = old.Status
						p.State = old.State
					}
					if !slices.Contains(fields, "owner") {
						p.Owner = old.Owner
					}
					if !slices.Contains(fields, "description") {
						p.Description = old.Description
					}
					if !slices.Contains(fields, "startDate") {
						p.StartDate = old.StartDate
					}
					if !slices.Contains(fields, "dueDate") {
						p.DueDate = old.DueDate
					}
					if !slices.Contains(fields, "url") {
						p.URL = old.URL
					}
					if e = cleanProject(p); e != nil {
						return nil, e
					}
				} else if !errors.Is(e, sql.ErrNoRows) {
					return nil, projectError(e)
				}
			}
		}
		result = append(result, rows...)
	}
	return result, nil
}
func (s *Service) ImportProjects(ctx context.Context, rows []domain.Project, requestID string) error {
	r, demo, e := s.projectContext(ctx)
	if e != nil {
		return e
	}
	if len(rows) == 0 || len(rows) > 2000 || len(requestID) < 10 || len(requestID) > 100 {
		return invalid("Нужен предпросмотр импорта от 1 до 2000 проектов")
	}
	seen := map[string]bool{}
	for i := range rows {
		rows[i].Sheet, rows[i].RemoteVersion, rows[i].Fields = nil, nil, nil
		rows[i].SourceVisibility = ""
		rows[i].SyncState, rows[i].SyncMessage = "", ""
		if e = cleanProject(&rows[i]); e != nil {
			return e
		}
		if rows[i].ID == "" || seen[rows[i].ID] {
			return invalid("Некорректные идентификаторы импорта")
		}
		seen[rows[i].ID] = true
	}
	return projectError(r.SaveProjects(ctx, demo, rows, requestID))
}
func (s *Service) CheckProjects(ctx context.Context) error {
	_, _, e := s.projectContext(ctx)
	return e
}

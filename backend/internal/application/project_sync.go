package application

import (
	"context"
	"encoding/json"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"time"
)

func (s *Service) WithProjectSource(source ports.ProjectSheetSource) *Service {
	s.projectSource = source
	return s
}
func (s *Service) ProjectDetail(ctx context.Context, id string) (domain.Project, error) {
	r, demo, e := s.projectContext(ctx)
	if e != nil {
		return domain.Project{}, e
	}
	sr, ok := r.(ports.ProjectSyncRepository)
	if !ok {
		return domain.Project{}, ErrNotFound
	}
	return sr.ProjectDetail(ctx, demo, id)
}
func (s *Service) BootstrapProjects(ctx context.Context, raw []byte) error {
	r, ok := s.repo.(ports.ProjectSyncRepository)
	if !ok {
		return nil
	}
	var book domain.ProjectSheetSnapshot
	if e := json.Unmarshal(raw, &book); e != nil {
		return e
	}
	projects, e := ParseProjectSheet(book)
	if e != nil {
		return e
	}
	return r.MergeProjectSheet(ctx, book, projects, true)
}
func (s *Service) ProjectSyncStatus(ctx context.Context) (domain.ProjectSyncStatus, error) {
	r, demo, e := s.projectContext(ctx)
	if e != nil {
		return domain.ProjectSyncStatus{}, e
	}
	if demo {
		return domain.ProjectSyncStatus{}, nil
	}
	sr, ok := r.(ports.ProjectSyncRepository)
	if !ok {
		return domain.ProjectSyncStatus{}, ErrNotFound
	}
	out, e := sr.ProjectSyncStatus(ctx)
	out.Configured = s.projectSource != nil && s.projectSource.Configured()
	return out, e
}
func (s *Service) SyncProjects(ctx context.Context) error {
	if !s.projectMu.TryLock() {
		return invalid("Синхронизация проектов уже выполняется")
	}
	defer s.projectMu.Unlock()
	if e := s.workspaceEnabled(ctx, "projects"); e != nil {
		return e
	}
	r, ok := s.repo.(ports.ProjectSyncRepository)
	if !ok || s.projectSource == nil || !s.projectSource.Configured() {
		return invalid("Подключите фоновую синхронизацию Google Таблицы")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	syncOnce := func() error {
		book, e := s.projectSource.Snapshot(ctx)
		if e != nil {
			return e
		}
		projects, e := ParseProjectSheet(book)
		if e != nil {
			return e
		}
		if e = r.MergeProjectSheet(ctx, book, projects, false); e != nil {
			return e
		}
		pending, e := r.PendingProjectChanges(ctx, book.SpreadsheetID)
		if e != nil {
			return e
		}
		for _, change := range pending {
			if e = s.projectSource.Apply(ctx, change); e != nil {
				return e
			}
		}
		if len(pending) > 0 {
			book, e = s.projectSource.Snapshot(ctx)
			if e != nil {
				return e
			}
			projects, e = ParseProjectSheet(book)
			if e != nil {
				return e
			}
			return r.MergeProjectSheet(ctx, book, projects, false)
		}
		return nil
	}
	e := syncOnce()
	if e != nil {
		_ = r.ProjectSyncError(context.WithoutCancel(ctx), e.Error())
	}
	return e
}
func (s *Service) RunProjectWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if s.projectSource != nil && s.projectSource.Configured() {
			_ = s.SyncProjects(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) ResolveProjectConflict(ctx context.Context, id string, version int, useRemote bool) error {
	if e := s.workspaceEnabled(ctx, "projects"); e != nil {
		return e
	}
	r, ok := s.repo.(ports.ProjectSyncRepository)
	if !ok {
		return ErrNotFound
	}
	return projectError(r.ResolveProjectConflict(ctx, id, version, useRemote))
}

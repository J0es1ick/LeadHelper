package application

import (
	"context"
	"database/sql"
	"errors"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"slices"
	"strings"
)

func (s *Service) viewCore(ctx context.Context) (domain.State, error) {
	if r, ok := s.repo.(ports.PagedMailRepository); ok {
		return r.ViewCore(ctx)
	}
	return s.repo.View(ctx)
}
func (s *Service) updateCore(ctx context.Context, fn func(*domain.State) error) error {
	if r, ok := s.repo.(ports.PagedMailRepository); ok {
		return r.UpdateCore(ctx, fn)
	}
	return s.repo.Update(ctx, fn)
}
func (s *Service) updateMail(ctx context.Context, scope domain.MailScope, fn func(*domain.State) error) error {
	if r, ok := s.repo.(ports.PagedMailRepository); ok {
		return r.UpdateMessages(ctx, scope, fn)
	}
	return s.repo.Update(ctx, fn)
}
func mailError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func (s *Service) mailContext(ctx context.Context) (domain.State, error) {
	st, e := s.viewCore(ctx)
	if e == nil {
		e = enabled(&st, "mail")
	}
	return st, e
}
func (s *Service) Message(ctx context.Context, id string) (domain.Message, error) {
	st, e := s.mailContext(ctx)
	if e != nil {
		return domain.Message{}, e
	}
	if r, ok := s.repo.(ports.PagedMailRepository); ok {
		m, e := r.MailMessage(ctx, id, st.Settings.Demo)
		return m, mailError(e)
	}
	for _, m := range st.Messages {
		if m.ID == id && m.Demo == st.Settings.Demo {
			return m, nil
		}
	}
	return domain.Message{}, ErrNotFound
}
func validateMailQuery(q domain.MailQuery) error {
	if !slices.Contains([]string{"inbox", "sent", "drafts", "archive", "trash", "starred", "all"}, q.Folder) {
		return invalid("Неизвестная папка")
	}
	if len(q.Search) > 300 || len(q.Cursor) > 3000 {
		return invalid("Слишком длинный запрос")
	}
	if q.Limit < 0 || q.Limit > 100 {
		return invalid("Размер страницы: от 1 до 100")
	}
	return nil
}
func (s *Service) MailPage(ctx context.Context, q domain.MailQuery) (domain.MailPage, error) {
	st, e := s.mailContext(ctx)
	if e != nil {
		return domain.MailPage{}, e
	}
	q.Search = strings.TrimSpace(q.Search)
	if e = validateMailQuery(q); e != nil {
		return domain.MailPage{}, e
	}
	if r, ok := s.repo.(ports.PagedMailRepository); ok {
		p, e := r.MailPage(ctx, st.Settings.Demo, q)
		if e != nil && strings.Contains(e.Error(), "Некорректная страница") {
			e = invalid(e.Error())
		}
		return p, e
	}
	return domain.MailPage{}, invalid("Перезапустите сервис для обновления почтового хранилища")
}
func (s *Service) MailThread(ctx context.Context, id string, q domain.MailQuery) (domain.MessagePage, error) {
	st, e := s.mailContext(ctx)
	if e != nil {
		return domain.MessagePage{}, e
	}
	if e = validateMailQuery(q); e != nil {
		return domain.MessagePage{}, e
	}
	if r, ok := s.repo.(ports.PagedMailRepository); ok {
		p, e := r.MailThread(ctx, st.Settings.Demo, id, q)
		if e != nil && strings.Contains(e.Error(), "Некорректная страница") {
			e = invalid(e.Error())
		}
		return p, mailError(e)
	}
	return domain.MessagePage{}, ErrNotFound
}
func (s *Service) PatchThread(ctx context.Context, id, folder string, p domain.MailPatch) error {
	st, e := s.mailContext(ctx)
	if e != nil {
		return e
	}
	if p.Folder != "" && !slices.Contains([]string{"inbox", "archive", "trash"}, p.Folder) {
		return invalid("Неизвестная папка")
	}
	if r, ok := s.repo.(ports.PagedMailRepository); ok {
		return mailError(r.PatchThread(ctx, st.Settings.Demo, id, folder, p))
	}
	return ErrNotFound
}

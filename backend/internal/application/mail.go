package application

import (
	"context"
	"encoding/base64"
	"errors"
	"leadhelper/internal/domain"
	"leadhelper/internal/mailbody"
	"net/mail"
	"slices"
	"strings"
	"time"
)

func validateMessage(m domain.Message, send bool) error {
	if len(m.Subject) > 300 || len(m.Body) > 200000 || len(m.HTML) > 1000000 || len(m.References) > 16000 || len(m.InReplyTo) > 1000 {
		return invalid("Письмо слишком большое")
	}
	for _, v := range []string{m.Subject, m.To, m.CC, m.InReplyTo, m.References} {
		if strings.ContainsAny(v, "\r\n") {
			return invalid("Недопустимый перенос строки в заголовке")
		}
	}
	if send && (strings.TrimSpace(m.Subject) == "" || (strings.TrimSpace(m.Body) == "" && strings.TrimSpace(m.HTML) == "")) {
		return invalid("Заполните тему и текст письма")
	}
	if send || m.To != "" {
		if _, e := mail.ParseAddressList(m.To); e != nil {
			return invalid("Проверьте адреса получателей")
		}
	}
	if m.CC != "" {
		if _, e := mail.ParseAddressList(m.CC); e != nil {
			return invalid("Проверьте адреса копии")
		}
	}
	total := 0
	for _, a := range m.Attachments {
		b, e := base64.StdEncoding.DecodeString(a.Data)
		if e != nil {
			return invalid("Повреждённое вложение")
		}
		total += len(b)
		if strings.TrimSpace(a.Name) == "" || len(a.Name) > 200 {
			return invalid("Некорректное имя вложения")
		}
	}
	if total > 5*1024*1024 || len(m.Attachments) > 10 {
		return invalid("Не более 10 вложений общим размером до 5 МБ")
	}
	return nil
}
func (s *Service) SaveDraft(ctx context.Context, id string, m domain.Message) (domain.Message, error) {
	if e := s.resolveDraftFiles(ctx, &m); e != nil {
		return m, e
	}
	m.AccountID = ""
	m.SourceFolder = ""
	m.UID = 0
	m.UIDValidity = 0
	m.MessageID = "<" + ID("leadhelper-") + "@localhost>"
	m.HTML = mailbody.Clean(m.HTML)
	m.FormatVersion = 3
	m.ContentHash = ""
	m.RawKey = ""
	m.ImportWarning = ""
	m.Raw = nil
	err := s.updateMail(ctx, domain.MailScope{IDs: []string{id}}, func(st *domain.State) error {
		if e := enabled(st, "mail"); e != nil {
			return e
		}
		if e := validateMessage(m, false); e != nil {
			return e
		}
		m.Folder = "drafts"
		m.Read = true
		m.Date = time.Now().UTC()
		m.Demo = st.Settings.Demo
		m.From = st.Settings.Email
		if !m.Demo {
			if p, ok := s.mail.(interface{ AccountID() string }); ok {
				m.AccountID = p.AccountID()
			}
		}
		if !m.Demo {
			if identity, ok := s.mail.(interface{ MailIdentity() (string, string) }); ok {
				email, _ := identity.MailIdentity()
				if email != "" {
					m.From = email
				}
			}
		}
		m.FromName = st.Settings.Name
		m.ExternalID = ""
		m.Delivery = ""
		if m.Attachments == nil {
			m.Attachments = []domain.Attachment{}
		}
		if id == "" {
			m.ID = ID("mail-")
			st.Messages = append(st.Messages, m)
			return nil
		}
		for i, old := range st.Messages {
			if old.ID == id && old.Demo == st.Settings.Demo {
				if old.Folder != "drafts" || old.Delivery == "sending" || old.Delivery == "uncertain" {
					return invalid("Письмо уже отправляется или не является черновиком")
				}
				m.ID = id
				if old.MessageID != "" {
					m.MessageID = old.MessageID
				}
				st.Messages[i] = m
				return nil
			}
		}
		return ErrNotFound
	})
	return m, err
}
func (s *Service) UpdateMessage(ctx context.Context, id, folder string, read, starred *bool) error {
	return s.updateMail(ctx, domain.MailScope{IDs: []string{id}}, func(st *domain.State) error {
		if e := enabled(st, "mail"); e != nil {
			return e
		}
		if folder != "" && !slices.Contains([]string{"inbox", "archive", "trash"}, folder) {
			return invalid("Неизвестная папка")
		}
		for i, m := range st.Messages {
			if m.ID == id && m.Demo == st.Settings.Demo {
				if m.Delivery == "sending" {
					return invalid("Дождитесь завершения отправки")
				}
				if folder != "" {
					st.Messages[i].Folder = folder
				}
				if read != nil {
					st.Messages[i].Read = *read
					if *read {
						for j := range st.Notifications {
							if st.Notifications[j].Provider == "mail" && st.Notifications[j].Target == id {
								st.Notifications[j].Read = true
							}
						}
					}
				}
				if starred != nil {
					st.Messages[i].Starred = *starred
				}
				return nil
			}
		}
		return ErrNotFound
	})
}
func (s *Service) Send(ctx context.Context, id string) (domain.Message, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	var m domain.Message
	e := s.updateMail(ctx, domain.MailScope{IDs: []string{id}}, func(st *domain.State) error {
		if e := enabled(st, "mail"); e != nil {
			return e
		}
		for i, old := range st.Messages {
			if old.ID == id && old.Demo == st.Settings.Demo {
				if old.Folder == "sent" {
					m = old
					return nil
				}
				if old.Folder != "drafts" {
					return invalid("Можно отправить только черновик")
				}
				if old.Delivery == "sending" || old.Delivery == "uncertain" {
					return invalid("Результат предыдущей отправки неизвестен. Проверьте папку отправленных у провайдера перед повторной отправкой")
				}
				if e := validateMessage(old, true); e != nil {
					return e
				}
				if !old.Demo {
					if s.mail == nil || !s.mail.Configured() {
						return invalid("Почта не подключена")
					}
					if e := s.mail.ValidateSend(old); e != nil {
						return invalid(e.Error())
					}
				}
				m = old
				st.Messages[i].Delivery = "sending"
				return nil
			}
		}
		return ErrNotFound
	})
	if e != nil {
		return m, e
	}
	if m.Folder == "sent" {
		return m, nil
	}
	m.Date = time.Now().UTC()
	if !m.Demo {
		if s.mail == nil || !s.mail.Configured() {
			e = invalid("Почта не подключена")
		} else {
			e = s.mail.Send(ctx, m)
		}
	}
	// Finish even if the client disconnects. An uncertain network outcome must never be retried automatically.
	saveErr := s.updateMail(context.Background(), domain.MailScope{IDs: []string{id}}, func(st *domain.State) error {
		for i := range st.Messages {
			if st.Messages[i].ID == id {
				if e != nil {
					var result interface{ DeliveryUncertain() bool }
					st.Messages[i].Delivery = "uncertain"
					if errors.As(e, &result) && !result.DeliveryUncertain() {
						st.Messages[i].Delivery = "rejected"
					}
				} else {
					st.Messages[i].Folder = "sent"
					st.Messages[i].Date = time.Now().UTC()
					st.Messages[i].Delivery = "accepted"
					if m.Demo {
						st.Messages[i].Delivery = "demo"
					}
				}
				m = st.Messages[i]
				return nil
			}
		}
		return ErrNotFound
	})
	if e != nil {
		if m.Delivery == "rejected" {
			return m, invalid("Письмо не отправлено. " + e.Error())
		}
		return m, invalid("Не удалось подтвердить отправку. Проверьте отправленные письма у провайдера перед повтором. " + e.Error())
	}
	if saveErr == nil && !m.Demo {
		if saver, ok := s.mail.(interface {
			SaveSent(context.Context, domain.Message) error
		}); ok {
			copyCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			err := saver.SaveSent(copyCtx, m)
			cancel()
			if err != nil {
				m.ImportWarning = "Письмо отправлено, но копию не удалось сохранить в папке Timeweb. Локальная копия сохранена; повторно отправлять письмо не нужно."
				_ = s.updateMail(context.Background(), domain.MailScope{IDs: []string{id}}, func(st *domain.State) error {
					for i := range st.Messages {
						if st.Messages[i].ID == id {
							st.Messages[i].ImportWarning = m.ImportWarning
						}
					}
					return nil
				})
			}
		}
	}
	return m, saveErr
}

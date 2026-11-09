package application

import (
	"context"
	"leadhelper/internal/domain"
	"time"
)

func (s *Service) ImportMessage(ctx context.Context, m domain.Message) (domain.Message, error) {
	err := s.updateMail(ctx, domain.MailScope{}, func(st *domain.State) error {
		if e := enabled(st, "mail"); e != nil {
			return e
		}
		if e := validateMessage(domain.Message{Subject: m.Subject, Body: m.Body, Attachments: m.Attachments}, false); e != nil {
			return e
		}
		m.ID = ID("mail-")
		m.ExternalID = ""
		m.Demo = st.Settings.Demo
		m.Folder = "inbox"
		m.Delivery = "imported"
		if m.Date.IsZero() {
			m.Date = time.Now().UTC()
		}
		st.Messages = append(st.Messages, m)
		return nil
	})
	return m, err
}

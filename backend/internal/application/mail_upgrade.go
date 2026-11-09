package application

import (
	"context"
	"fmt"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
)

// UpgradeMail re-decodes durable originals without moving IMAP cursors or changing
// local flags. The new index is committed atomically after its blobs are durable.
func (s *Service) UpgradeMail(ctx context.Context, decode func([]byte) (domain.Message, error)) error {
	if r, ok := s.repo.(ports.PagedMailRepository); ok {
		for {
			ids, e := r.PendingMail(ctx, 3, 100)
			if e != nil {
				return e
			}
			if len(ids) == 0 {
				return nil
			}
			if e = s.updateMail(ctx, domain.MailScope{IDs: ids}, func(st *domain.State) error { return s.upgradeMessages(ctx, st, decode) }); e != nil {
				return e
			}
		}
	}
	st, err := s.viewCore(ctx)
	if err != nil {
		return err
	}
	pending := false
	for _, m := range st.Messages {
		if m.RawKey != "" && m.FormatVersion < 3 {
			pending = true
			break
		}
	}
	if !pending {
		return nil
	}
	return s.updateCore(ctx, func(st *domain.State) error { return s.upgradeMessages(ctx, st, decode) })
}
func (s *Service) upgradeMessages(ctx context.Context, st *domain.State, decode func([]byte) (domain.Message, error)) error {
	for i := range st.Messages {
		m := &st.Messages[i]
		if m.RawKey == "" || m.FormatVersion >= 3 {
			continue
		}
		data, err := s.RawMessage(ctx, *m)
		if err != nil {
			return fmt.Errorf("read original %s: %w", m.ID, err)
		}
		parsed, err := decode(data)
		if err != nil {
			m.FormatVersion = 3
			continue
		} // Retain malformed messages as imported.
		if err = s.storeMessage(ctx, &parsed); err != nil {
			return err
		}
		used := map[string]bool{}
		for j := range parsed.Attachments {
			a := &parsed.Attachments[j]
			a.ID = ""
			for _, old := range m.Attachments {
				if !used[old.ID] && old.Name == a.Name && ((a.BlobKey != "" && a.BlobKey == old.BlobKey) || (a.Data != "" && a.Data == old.Data)) {
					a.ID = old.ID
					used[old.ID] = true
					break
				}
			}
			if a.ID == "" {
				a.ID = ID("file-")
			}
		}
		m.HTML = parsed.HTML
		m.InReplyTo = parsed.InReplyTo
		m.References = parsed.References
		m.MessageID = parsed.MessageID
		m.Attachments = parsed.Attachments
		m.Body = parsed.Body
		m.FormatVersion = 3
	}
	return nil
}

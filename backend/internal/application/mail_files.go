package application

import (
	"context"
	"encoding/base64"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
)

func (s *Service) AttachmentBytes(ctx context.Context, a domain.Attachment) ([]byte, error) {
	if a.BlobKey != "" {
		if s.blobs == nil {
			return nil, invalid("Хранилище вложений не настроено")
		}
		return s.blobs.Get(ctx, a.BlobKey)
	}
	return base64.StdEncoding.DecodeString(a.Data)
}
func (s *Service) RawMessage(ctx context.Context, m domain.Message) ([]byte, error) {
	if m.RawKey != "" && s.blobs != nil {
		return s.blobs.Get(ctx, m.RawKey)
	}
	return nil, nil
}
func (s *Service) MessageWithFiles(ctx context.Context, m domain.Message) (domain.Message, error) {
	for i := range m.Attachments {
		data, e := s.AttachmentBytes(ctx, m.Attachments[i])
		if e != nil {
			return m, e
		}
		m.Attachments[i].Data = base64.StdEncoding.EncodeToString(data)
	}
	return m, nil
}

// A browser may forward an attachment only if it belongs to a visible message.
func (s *Service) resolveDraftFiles(ctx context.Context, m *domain.Message) error {
	st, e := s.State(ctx)
	if e != nil {
		return e
	}
	for i := range m.Attachments {
		a := &m.Attachments[i]
		if a.BlobKey == "" {
			continue
		}
		allowed := false
		if r, ok := s.repo.(ports.PagedMailRepository); ok {
			var err error
			allowed, err = r.OwnsAttachment(ctx, a.BlobKey, st.Settings.Demo)
			if err != nil {
				return err
			}
		}
		for _, source := range st.Messages {
			for _, file := range source.Attachments {
				if file.BlobKey == a.BlobKey && file.BlobKey != "" {
					allowed = true
				}
			}
		}
		if !allowed {
			return invalid("Вложение недоступно")
		}
		data, e := s.AttachmentBytes(ctx, *a)
		if e != nil {
			return e
		}
		a.Data = base64.StdEncoding.EncodeToString(data)
		a.Size = int64(len(data))
		a.BlobKey = ""
	}
	return nil
}

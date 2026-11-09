package application

import (
	"context"
	"encoding/base64"
	"fmt"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"time"
)

func (s *Service) syncMail(ctx context.Context, provider ports.IncrementalMailProvider) (domain.SyncLog, error) {
	select {
	case s.mailSyncGate <- struct{}{}:
		defer func() { <-s.mailSyncGate }()
	case <-ctx.Done():
		return domain.SyncLog{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return domain.SyncLog{}, err
	}
	st, e := s.viewCore(ctx)
	if e != nil {
		return domain.SyncLog{}, e
	}
	if e = enabled(&st, "mail"); e != nil {
		return domain.SyncLog{}, e
	}
	l := domain.SyncLog{ID: ID("sync-"), Provider: "mail", At: time.Now().UTC(), Demo: st.Settings.Demo, Success: true}
	if st.Settings.Demo {
		l.Message = "Демонстрационная почта. Подключение к серверу не выполнялось"
		return l, nil
	}
	if !provider.Configured() {
		return l, invalid("Укажите IMAP_HOST, IMAP_USER и пароль на сервере, затем перезапустите сервис")
	}
	now := time.Now().UTC()
	e = s.updateCore(ctx, func(current *domain.State) error {
		if current.Settings.Demo {
			return invalid("Деморежим включён; загрузка приостановлена")
		}
		if err := enabled(current, "mail"); err != nil {
			return err
		}
		last := current.MailSync.LastSuccessAt
		if current.MailSync.AccountID != provider.AccountID() {
			last = nil
		}
		current.MailSync = domain.MailSyncStatus{AccountID: provider.AccountID(), State: "syncing", StartedAt: &now, UpdatedAt: &now, LastSuccessAt: last}
		return nil
	})
	if e != nil {
		return l, e
	}
	imported := 0
	e = provider.Sync(ctx, st.MailCheckpoints, func(batch domain.MailBatch) error {
		// Durable blobs precede the atomic index + checkpoint commit. An interruption
		// may leave an unused blob, but can never leave a cursor pointing past data.
		for k := range batch.Messages {
			if err := s.storeMessage(ctx, &batch.Messages[k]); err != nil {
				return err
			}
		}
		count := 0
		err := s.updateMail(ctx, domain.MailScope{Batch: batch.Messages}, func(current *domain.State) error {
			if current.Settings.Demo {
				return invalid("Деморежим включён; загрузка приостановлена")
			}
			if err := enabled(current, "mail"); err != nil {
				return err
			}
			for _, m := range batch.Messages {
				found := -1
				for j, old := range current.Messages {
					if old.Demo || old.AccountID != m.AccountID || old.SourceFolder != m.SourceFolder {
						continue
					}
					// Content match is only for an epoch change. Identical copies with separate
					// UIDs in the same epoch remain separate messages, as on the mail server.
					if old.ExternalID == m.ExternalID || (old.UIDValidity != m.UIDValidity && m.ContentHash != "" && old.ContentHash == m.ContentHash) {
						found = j
						break
					}
				}
				if found < 0 {
					current.Messages = append(current.Messages, m)
					count++
					previous, known := st.MailCheckpoints[batch.Key]
					if st.MailSync.LastSuccessAt != nil && st.MailSync.AccountID == provider.AccountID() && known && previous.UIDValidity == batch.Checkpoint.UIDValidity && m.Folder == "inbox" && !m.Read {
						addNotification(current, domain.Notification{ID: "mail:" + m.ExternalID, Provider: "mail", Title: m.FromName, Text: clipText(m.Subject, 200), Target: m.ID, At: time.Now().UTC()})
					}
				} else {
					old := current.Messages[found]
					m.ID = old.ID
					m.Folder = old.Folder
					m.Read = old.Read
					m.Starred = old.Starred
					current.Messages[found] = m
				}
			}
			if current.MailCheckpoints == nil {
				current.MailCheckpoints = map[string]domain.MailCheckpoint{}
			}
			current.MailCheckpoints[batch.Key] = batch.Checkpoint
			updated := time.Now().UTC()
			current.MailSync.Folder = batch.Checkpoint.Folder
			current.MailSync.Imported += count
			current.MailSync.UpdatedAt = &updated
			return nil
		})
		if err == nil {
			imported += count
		}
		return err
	})
	l.Count = imported
	l.Success = e == nil
	l.Message = fmt.Sprintf("Синхронизация завершена. Новых писем: %d", imported)
	if e != nil {
		l.Message = e.Error()
	}
	// Persist a cancelled run's status too; its already committed batches survive.
	saveErr := s.updateCore(context.Background(), func(current *domain.State) error {
		ended := time.Now().UTC()
		current.MailSync.UpdatedAt = &ended
		if e == nil {
			current.MailSync.State = "idle"
			current.MailSync.LastSuccessAt = &ended
			current.MailSync.Error = ""
		} else {
			current.MailSync.State = "error"
			current.MailSync.Error = l.Message
		}
		current.SyncLogs = append([]domain.SyncLog{l}, current.SyncLogs...)
		if len(current.SyncLogs) > 100 {
			current.SyncLogs = current.SyncLogs[:100]
		}
		return nil
	})
	return l, saveErr
}

func (s *Service) storeMessage(ctx context.Context, m *domain.Message) error {
	if s.blobs == nil {
		m.Raw = nil
		return nil
	}
	if len(m.Raw) > 0 {
		key, e := s.blobs.Put(ctx, m.Raw)
		if e != nil {
			return e
		}
		m.RawKey = key
		m.Raw = nil
	}
	for k := range m.Attachments {
		a := &m.Attachments[k]
		if a.Data == "" {
			continue
		}
		data, e := base64.StdEncoding.DecodeString(a.Data)
		if e != nil {
			return e
		}
		key, e := s.blobs.Put(ctx, data)
		if e != nil {
			return e
		}
		a.BlobKey = key
		a.Size = int64(len(data))
		a.Data = ""
	}
	return nil
}

// RunMailWorker is tied to the server lifetime, not a browser tab. Startup always
// checks immediately; persisted UID checkpoints handle arbitrarily long downtime.
func (s *Service) RunMailWorker(ctx context.Context, interval time.Duration) {
	if st, e := s.viewCore(ctx); e == nil && st.MailSync.State == "syncing" {
		_ = s.updateCore(ctx, func(current *domain.State) error {
			current.MailSync.State = "error"
			current.MailSync.Error = "Предыдущая проверка была прервана. Загрузка продолжится после подключения почты"
			return nil
		})
	}
	if interval < time.Second {
		interval = time.Minute
	}
	delay := interval
	for {
		if ctx.Err() != nil {
			return
		}
		st, err := s.viewCore(ctx)
		if err == nil && !st.Settings.Demo && st.Settings.Modules["mail"] && s.mail != nil && s.mail.Configured() {
			run, cancel := context.WithTimeout(ctx, 5*time.Minute)
			l, e := s.Sync(run, "mail")
			cancel()
			if e != nil || !l.Success {
				delay = min(max(delay*2, interval), 15*time.Minute)
			} else {
				delay = interval
			}
		} else {
			delay = interval
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

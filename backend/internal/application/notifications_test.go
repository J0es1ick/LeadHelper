package application

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"testing"
)

func TestMailNotificationsSkipInitialHistoryDeduplicateAndSurviveRead(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	_ = repo.Update(ctx, func(st *domain.State) error { st.Settings.Demo = false; return nil })
	p := &resumableMail{run: func(_ context.Context, _ map[string]domain.MailCheckpoint, save func(domain.MailBatch) error) error {
		if e := save(batch(1, 100, "first")); e != nil {
			return e
		}
		return save(batch(2, 100, "second"))
	}}
	s.mail = p
	if _, e := s.Sync(ctx, "mail"); e != nil {
		t.Fatal(e)
	}
	st, _ := s.State(ctx)
	if len(st.Notifications) != 0 {
		t.Fatal("initial history triggered notifications")
	}
	p.run = func(_ context.Context, _ map[string]domain.MailCheckpoint, save func(domain.MailBatch) error) error {
		return save(batch(3, 100, "new"))
	}
	_, _ = s.Sync(ctx, "mail")
	st, _ = s.State(ctx)
	if len(st.Notifications) != 1 {
		t.Fatal("new arrival missing")
	}
	id := st.Notifications[0].ID
	_ = s.ReadNotifications(ctx, id)
	_, _ = s.Sync(ctx, "mail")
	st, _ = s.State(ctx)
	if len(st.Notifications) != 1 || !st.Notifications[0].Read || st.NotificationCursors != nil {
		t.Fatal("dedupe/read state failed")
	}
	p.run = func(_ context.Context, _ map[string]domain.MailCheckpoint, save func(domain.MailBatch) error) error {
		return save(batch(4, 101, "epoch-new"))
	}
	_, _ = s.Sync(ctx, "mail")
	st, _ = s.State(ctx)
	if len(st.Notifications) != 1 {
		t.Fatal("UID reset triggered historical notification")
	}
}

type workspaceFixture struct {
	page domain.RemotePage
	err  error
}

func (*workspaceFixture) Configured() bool  { return true }
func (*workspaceFixture) AccountID() string { return "bitrix-test" }
func (w *workspaceFixture) Browse(context.Context, domain.RemoteQuery) (domain.RemotePage, error) {
	return w.page, w.err
}
func (*workspaceFixture) Calendar(context.Context, string, string) (domain.CalendarPage, error) {
	return domain.CalendarPage{}, nil
}
func (*workspaceFixture) SendChat(context.Context, string, string) error { return nil }
func TestBitrixNotificationBaselineFailureAndOwnMessage(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	_ = repo.Update(ctx, func(st *domain.State) error { st.Settings.Demo = false; return nil })
	p := &workspaceFixture{page: domain.RemotePage{Items: []domain.RemoteItem{{ID: "chat1", MessageID: "100", Unread: 1}}}}
	s.bitrix = p
	_, _ = s.Sync(ctx, "bitrix")
	st, _ := s.State(ctx)
	if len(st.Notifications) != 0 {
		t.Fatal("historical chat alerted")
	}
	p.page.Items[0].MessageID = "101"
	p.err = errors.New("offline")
	_, _ = s.Sync(ctx, "bitrix")
	raw, _ := repo.View(ctx)
	if raw.NotificationCursors["bitrix-test:messages"] != "100" {
		t.Fatal("failed sync advanced baseline")
	}
	p.err = nil
	_, _ = s.Sync(ctx, "bitrix")
	st, _ = s.State(ctx)
	if len(st.Notifications) != 1 {
		t.Fatal("incoming chat missing")
	}
	p.page.Items[0].MessageID = "102"
	p.page.Items[0].Own = true
	_, _ = s.Sync(ctx, "bitrix")
	st, _ = s.State(ctx)
	if len(st.Notifications) != 1 {
		t.Fatal("own message alerted")
	}
}

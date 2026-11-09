package application

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"testing"
)

type readableWorkspace struct {
	workspaceFixture
	readErr error
	calls   int
	readID  string
}

func (p *readableWorkspace) ReadChat(_ context.Context, chat, id string) (domain.ChatRead, error) {
	p.calls++
	p.readID = id
	return domain.ChatRead{MessageID: id, Unread: 1}, p.readErr
}
func TestChatReadNotificationsOnlyConfirmedViewedMessages(t *testing.T) {
	s, r := fixture(t)
	ctx := context.Background()
	p := &readableWorkspace{}
	s.bitrix = p
	_ = r.Update(ctx, func(st *domain.State) error {
		st.Settings.Demo = false
		st.Notifications = []domain.Notification{
			{ID: "bitrix-test:messages:100", Provider: "bitrix", Target: "chat1"},
			{ID: "bitrix-test:messages:101", Provider: "bitrix", Target: "chat1"},
			{ID: "bitrix-test:messages:102", Provider: "bitrix", Target: "chat1"},
			{ID: "bitrix-test:messages:99", Provider: "bitrix", Target: "chat2"},
			{ID: "other-account:messages:100", Provider: "bitrix", Target: "chat1"},
		}
		return nil
	})
	if _, e := s.ReadChat(ctx, "chat1", "101", "other-account"); e == nil || p.calls != 0 {
		t.Fatal("account guard failed")
	}
	p.readErr = errors.New("offline")
	if _, e := s.ReadChat(ctx, "chat1", "101", "bitrix-test"); e == nil {
		t.Fatal("failed provider accepted")
	}
	st, _ := r.View(ctx)
	if st.Notifications[0].Read {
		t.Fatal("failure hid notification")
	}
	p.readErr = nil
	if _, e := s.ReadChat(ctx, "chat1", "101", "bitrix-test"); e != nil {
		t.Fatal(e)
	}
	st, _ = r.View(ctx)
	for i, n := range st.Notifications {
		if n.Read != (i < 2) {
			t.Fatal("wrong notification consumed", n)
		}
	}
	if p.readID != "101" {
		t.Fatal("read past viewed message")
	}
	// An external read reconciles the remaining notification without touching others.
	p.page = domain.RemotePage{AccountID: "bitrix-test", Items: []domain.RemoteItem{{ID: "chat1", MessageID: "102", Unread: 0}}}
	if _, e := s.Browse(ctx, "bitrix", domain.RemoteQuery{Kind: "chats"}); e != nil {
		t.Fatal(e)
	}
	st, _ = r.View(ctx)
	if !st.Notifications[2].Read || st.Notifications[3].Read || st.Notifications[4].Read {
		t.Fatal("remote read reconciliation failed")
	}
}
func TestMailMessageReadConsumesOnlyOwnNotification(t *testing.T) {
	s, r := fixture(t)
	ctx := context.Background()
	_ = r.Update(ctx, func(st *domain.State) error {
		st.Settings.Demo = false
		st.Messages = []domain.Message{{ID: "m1", Folder: "inbox"}, {ID: "m2", Folder: "inbox"}}
		st.Notifications = []domain.Notification{{ID: "n1", Provider: "mail", Target: "m1"}, {ID: "n2", Provider: "mail", Target: "m2"}}
		return nil
	})
	read := true
	if e := s.UpdateMessage(ctx, "m1", "", &read, nil); e != nil {
		t.Fatal(e)
	}
	st, _ := r.View(ctx)
	if !st.Notifications[0].Read || st.Notifications[1].Read {
		t.Fatal(st.Notifications)
	}
}

package storage

import (
	"context"
	"leadhelper/internal/domain"
	"testing"
	"time"
)

func TestMailThreadReadConsumesNotificationsAtomicallyAndKeepsNewArrivals(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	r := sqlFixture(t, []domain.Message{
		{ID: "a", MessageID: "<a@test>", Subject: "Thread", Folder: "inbox", Date: now},
		{ID: "b", MessageID: "<b@test>", InReplyTo: "<a@test>", Subject: "Re: Thread", Folder: "inbox", Date: now.Add(time.Minute)},
		{ID: "other", MessageID: "<other@test>", Subject: "Other", Folder: "inbox", Date: now},
		{ID: "old", MessageID: "<old@test>", Subject: "Old", Folder: "inbox", Date: now, Read: true},
	})
	_ = r.UpdateCore(ctx, func(st *domain.State) error {
		for _, id := range []string{"a", "b", "other", "old"} {
			st.Notifications = append(st.Notifications, domain.Notification{ID: id, Provider: "mail", Target: id})
		}
		return nil
	})
	st, e := r.ViewCore(ctx)
	if e != nil || !st.Notifications[3].Read {
		t.Fatal("old read notification not reconciled", e)
	}
	shell, e := r.ViewShell(ctx)
	if e != nil || !shell.Notifications[3].Read {
		t.Fatal("workspace shell did not reconcile old read", e)
	}
	read := true
	if e = r.PatchThread(ctx, false, "a", "inbox", domain.MailPatch{Read: &read}); e != nil {
		t.Fatal(e)
	}
	st, e = r.ViewCore(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if !st.Notifications[0].Read || !st.Notifications[1].Read || st.Notifications[2].Read {
		t.Fatal(st.Notifications)
	}
	// Re-marking a conversation unread must not resurrect consumed alerts.
	read = false
	if e = r.PatchThread(ctx, false, "a", "inbox", domain.MailPatch{Read: &read}); e != nil {
		t.Fatal(e)
	}
	st, _ = r.ViewCore(ctx)
	if !st.Notifications[0].Read {
		t.Fatal("read state was not committed with thread")
	}
	if e = r.UpdateMessages(ctx, domain.MailScope{}, func(st *domain.State) error {
		st.Messages = append(st.Messages, domain.Message{ID: "new", MessageID: "<new@test>", InReplyTo: "<a@test>", Subject: "Re: Thread", Folder: "inbox", Date: now.Add(2 * time.Minute)})
		st.Notifications = append(st.Notifications, domain.Notification{ID: "new", Provider: "mail", Target: "new"})
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	st, _ = r.ViewCore(ctx)
	if st.Notifications[4].Read {
		t.Fatal("new arrival consumed by old thread read")
	}
	if e = r.PatchThread(ctx, false, "missing", "inbox", domain.MailPatch{Read: &read}); e == nil {
		t.Fatal("missing thread accepted")
	}
	if e = r.PatchThread(ctx, false, "old", "inbox", domain.MailPatch{Read: &read}); e != nil {
		t.Fatal(e)
	}
	st, _ = r.ViewCore(ctx)
	if !st.Notifications[3].Read {
		t.Fatal("historical consumed notification resurrected")
	}
}

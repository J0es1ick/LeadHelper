package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sqlFixture(t *testing.T, messages []domain.Message) *SQLiteRepository {
	t.Helper()
	dir := t.TempDir()
	legacy := filepath.Join(dir, "state.json")
	st := domain.State{Version: 1, Settings: domain.Settings{Email: "me@example.test", Modules: map[string]bool{"mail": true}}, Messages: messages, MailCheckpoints: map[string]domain.MailCheckpoint{"box": {LastUID: 8, UIDValidity: 1}}}
	data, _ := json.Marshal(st)
	if e := os.WriteFile(legacy, data, 0600); e != nil {
		t.Fatal(e)
	}
	r, e := OpenSQLite(filepath.Join(dir, "mail.sqlite"), legacy, domain.State{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	return r
}
func TestSQLitePagesThreadsAndSearch(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	a := domain.Message{ID: "a", MessageID: "<a@test>", Subject: "Work", From: "peer@example.test", To: "me@example.test", Folder: "inbox", Date: at, Body: "needleword hidden in oldest reply", HTML: "<b>private body</b>", Attachments: []domain.Attachment{{ID: "img", Name: "img", Data: "c2VjcmV0", BlobKey: "blob"}}}
	b := a
	b.ID = "b"
	b.MessageID = "<b@test>"
	b.InReplyTo = a.MessageID
	b.Folder = "sent"
	b.Date = at.Add(time.Hour)
	b.Read = true
	b.Body = "reply"
	c := a
	c.ID = "c"
	c.MessageID = "<c@test>"
	c.References = "<a@test> <b@test>"
	c.Date = at.Add(2 * time.Hour)
	c.Body = "latest"
	duplicate := c
	duplicate.ID = "copy"
	duplicate.Folder = "archive"
	ms := []domain.Message{a, b, c, duplicate}
	for i := 0; i < 70; i++ {
		ms = append(ms, domain.Message{ID: fmt.Sprintf("separate-%03d", i), MessageID: fmt.Sprintf("<n%d@test>", i), Subject: "Repeated subject", Folder: "inbox", Date: at.Add(-time.Hour), Body: "ordinary", Read: true})
	}
	r := sqlFixture(t, ms)
	core, e := r.ViewCore(ctx)
	if e != nil || len(core.Messages) != 0 {
		t.Fatal("core eagerly loads mailbox", e)
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		page, e := r.MailPage(ctx, false, domain.MailQuery{Folder: "inbox", Limit: 25, Cursor: cursor})
		if e != nil {
			t.Fatal(e)
		}
		if len(page.Items) > 25 {
			t.Fatal("page not bounded")
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate across cursor pages")
			}
			seen[item.ID] = true
			if item.Latest.HTML != "" || len([]rune(item.Latest.Body)) > 220 {
				t.Fatal("full body in list")
			}
			for _, a := range item.Latest.Attachments {
				if a.Data != "" {
					t.Fatal("base64 in list")
				}
			}
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 71 {
		t.Fatalf("got %d threads", len(seen))
	}
	p, e := r.MailPage(ctx, false, domain.MailQuery{Folder: "inbox", Search: "needleword", Limit: 25})
	if e != nil || len(p.Items) != 1 {
		t.Fatal("whole-thread search failed", e, len(p.Items))
	}
	thread := p.Items[0]
	if thread.Count != 3 {
		t.Fatalf("duplicate server copy counted: %d", thread.Count)
	}
	history, e := r.MailThread(ctx, false, thread.ID, domain.MailQuery{Folder: "inbox", Limit: 1})
	if e != nil || len(history.Items) != 1 || history.Items[0].Body != "latest" || history.NextCursor == "" {
		t.Fatal("latest-first/history paging failed", e, history)
	}
	full, e := r.MailMessage(ctx, "a", false)
	if e != nil || full.HTML == "" {
		t.Fatal("body missing on demand", e)
	}
	yes := true
	if e = r.PatchThread(ctx, false, "c", "inbox", domain.MailPatch{Read: &yes}); e != nil {
		t.Fatal(e)
	}
	p, e = r.MailPage(ctx, false, domain.MailQuery{Folder: "inbox", Search: "needleword"})
	if e != nil || p.Items[0].Unread != 0 {
		t.Fatal("thread read flags failed", e)
	}
	full, e = r.MailMessage(ctx, "a", false)
	if e != nil || !full.Read {
		t.Fatal("stored JSON flags invalid", e)
	}
	if e = r.PatchThread(ctx, false, thread.ID, "inbox", domain.MailPatch{Folder: "trash"}); e != nil {
		t.Fatal(e)
	}
	p, e = r.MailPage(ctx, false, domain.MailQuery{Folder: "inbox", Search: "needleword"})
	if e != nil || len(p.Items) != 0 {
		t.Fatal("trashed thread visible")
	}
	p, e = r.MailPage(ctx, false, domain.MailQuery{Folder: "trash"})
	if e != nil || len(p.Items) != 1 {
		t.Fatal("trash lost thread", e)
	}
	if _, e = r.MailPage(ctx, false, domain.MailQuery{Folder: "trash", Cursor: cursor}); e == nil {
		t.Fatal("cursor from another folder accepted")
	}
}
func TestSQLiteAtomicMailAndCheckpoint(t *testing.T) {
	ctx := context.Background()
	r := sqlFixture(t, []domain.Message{{ID: "mail", Folder: "inbox", MessageID: "<a@test>", Date: time.Now()}})
	sentinel := errors.New("rollback")
	e := r.UpdateMessages(ctx, domain.MailScope{IDs: []string{"mail"}}, func(st *domain.State) error {
		st.Messages[0].Read = true
		st.MailCheckpoints["box"] = domain.MailCheckpoint{LastUID: 99}
		return sentinel
	})
	if !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	core, _ := r.ViewCore(ctx)
	m, _ := r.MailMessage(ctx, "mail", false)
	if m.Read || core.MailCheckpoints["box"].LastUID != 8 {
		t.Fatal("partial commit")
	}
	e = r.UpdateMessages(ctx, domain.MailScope{IDs: []string{"mail"}}, func(st *domain.State) error {
		st.Messages[0].Read = true
		st.MailCheckpoints["box"] = domain.MailCheckpoint{LastUID: 99}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	core, _ = r.ViewCore(ctx)
	m, _ = r.MailMessage(ctx, "mail", false)
	if !m.Read || core.MailCheckpoints["box"].LastUID != 99 {
		t.Fatal("atomic commit lost")
	}
	if _, e = r.MailMessage(ctx, "mail", true); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("cross-mode read")
	}
}

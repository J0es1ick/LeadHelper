package application

import (
	"context"
	"encoding/base64"
	"leadhelper/internal/adapters/mailformat"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/domain"
	"reflect"
	"testing"
	"time"
)

func TestUpgradeMailPreservesLocalStateAndCursors(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	s.WithBlobs(&storage.Blobs{Directory: t.TempDir()})
	file := domain.Attachment{ID: "stable-file", Name: "photo.png", ContentType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("photo")), ContentID: "photo", Inline: true}
	original := domain.Message{From: "a@example.test", To: "b@example.test", Subject: "Re: Hello", Body: "Hello", HTML: `<b>Hello</b><img src="cid:photo">`, MessageID: "<2@local>", InReplyTo: "<1@local>", References: "<1@local>", Date: time.Now(), Attachments: []domain.Attachment{file}}
	raw, e := mailformat.Encode(original)
	if e != nil {
		t.Fatal(e)
	}
	old := original
	old.ID = "stable-mail"
	old.HTML = ""
	old.InReplyTo = ""
	old.References = ""
	old.Folder = "archive"
	old.Read = true
	old.Starred = true
	old.Raw = raw
	old.Attachments[0].Inline = false
	old.Attachments[0].ContentID = ""
	if e = s.storeMessage(ctx, &old); e != nil {
		t.Fatal(e)
	}
	cursor := map[string]domain.MailCheckpoint{"box": {LastUID: 42, UIDValidity: 7}}
	repo.Update(ctx, func(st *domain.State) error {
		st.Messages = []domain.Message{old}
		st.MailCheckpoints = cursor
		return nil
	})
	if e = s.UpgradeMail(ctx, mailformat.Decode); e != nil {
		t.Fatal(e)
	}
	st, _ := repo.View(ctx)
	m := st.Messages[0]
	if m.ID != old.ID || m.Folder != "archive" || !m.Read || !m.Starred || m.RawKey != old.RawKey || m.Attachments[0].ID != "stable-file" || !reflect.DeepEqual(st.MailCheckpoints, cursor) {
		t.Fatal("upgrade changed identity, flags, attachment IDs or checkpoint")
	}
	if m.HTML == "" || m.InReplyTo != "<1@local>" || !m.Attachments[0].Inline || m.FormatVersion != 3 {
		t.Fatalf("missing rich fields %+v", m)
	}
	if e = s.UpgradeMail(ctx, func([]byte) (domain.Message, error) { t.Fatal("upgrade repeated"); return domain.Message{}, nil }); e != nil {
		t.Fatal(e)
	}
}

type identityMail struct{ mockMail }

func (identityMail) MailIdentity() (string, string) {
	return "connected@example.test", "imap.example.test"
}

func TestConnectedMailIdentityAndDraftHeaders(t *testing.T) {
	_, repo := fixture(t)
	s := New(repo, nil, &identityMail{})
	ctx := context.Background()
	st, _ := s.State(ctx)
	cfg := st.Settings
	cfg.Demo = false
	s.Settings(ctx, cfg)
	st, _ = s.State(ctx)
	if st.MailConnection.Email != "connected@example.test" || st.MailConnection.Host != "imap.example.test" {
		t.Fatal("missing connection identity")
	}
	m, e := s.SaveDraft(ctx, "", domain.Message{To: "you@example.test", Subject: "Re: Hello", Body: "Hello", HTML: `<b>Hello</b><script>bad()</script>`, InReplyTo: "<first@local>", References: "<first@local>"})
	if e != nil {
		t.Fatal(e)
	}
	if m.From != "connected@example.test" || m.MessageID == "" || m.InReplyTo != "<first@local>" || m.HTML != "<b>Hello</b>" {
		t.Fatalf("draft incorrect: %+v", m)
	}
	again, e := s.SaveDraft(ctx, m.ID, m)
	if e != nil || again.MessageID != m.MessageID {
		t.Fatal("draft message ID changed")
	}
	m.InReplyTo = "<id>\r\nBcc: other@example.test"
	if _, e = s.SaveDraft(ctx, m.ID, m); e == nil {
		t.Fatal("header injection accepted")
	}
}

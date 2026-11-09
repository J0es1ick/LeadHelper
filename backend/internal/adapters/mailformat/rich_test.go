package mailformat

import (
	"encoding/base64"
	"leadhelper/internal/domain"
	"strings"
	"testing"
	"time"
)

func TestRichReplyRoundTrip(t *testing.T) {
	m := domain.Message{From: "me@example.test", To: "you@example.test", Subject: "Re: Work", Date: time.Now(), Body: "Hello picture", HTML: `<p>Hello <b>picture</b><img src="cid:photo@local"></p>`, MessageID: "<reply@local>", InReplyTo: "<original@local>", References: "<root@local> <original@local>", Attachments: []domain.Attachment{{Name: "photo.png", ContentType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("image")), ContentID: "photo@local", Inline: true}, {Name: "notes.txt", ContentType: "text/plain", Data: base64.StdEncoding.EncodeToString([]byte("notes"))}}}
	raw, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.HTML != m.HTML || got.Body != m.Body || got.MessageID != m.MessageID || got.InReplyTo != m.InReplyTo || got.References != m.References {
		t.Fatalf("lost rich message: %+v", got)
	}
	if len(got.Attachments) != 2 || !got.Attachments[0].Inline || got.Attachments[0].ContentID != "photo@local" || got.Attachments[0].ContentType != "image/png" || got.Attachments[1].Inline {
		t.Fatalf("lost MIME metadata: %+v", got.Attachments)
	}
	if !strings.Contains(string(raw), "multipart/related") || !strings.Contains(string(raw), "multipart/alternative") {
		t.Fatal("missing related/alternative MIME")
	}
}
func TestCIDImageWithoutFilename(t *testing.T) {
	raw := "Subject: Inline\r\nContent-Type: multipart/related; boundary=x\r\n\r\n--x\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Photo<img src=\"cid:photo\"></p>\r\n--x\r\nContent-Type: image/png\r\nContent-ID: <photo>\r\nContent-Transfer-Encoding: base64\r\n\r\naW1hZ2U=\r\n--x--\r\n"
	m, e := Decode([]byte(raw))
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].ContentID != "photo" || !strings.Contains(m.HTML, "cid:photo") || !strings.Contains(m.Body, "Photo") {
		t.Fatalf("inline image lost: %+v", m)
	}
}

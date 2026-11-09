package mailformat

import (
	"encoding/base64"
	"leadhelper/internal/domain"
	"strings"
	"testing"
	"time"
)

func TestMIMERoundTrip(t *testing.T) {
	in := domain.Message{From: "lead@example.test", To: "test@example.test", Subject: "Тестовое письмо", Body: "Привет!\nТекст письма.", Date: time.Now(), Attachments: []domain.Attachment{{ID: "a", Name: "привет.txt", Data: base64.StdEncoding.EncodeToString([]byte("данные"))}}}
	data, e := Encode(in)
	if e != nil {
		t.Fatal(e)
	}
	out, e := Decode(data)
	if e != nil {
		t.Fatal(e)
	}
	if out.Subject != in.Subject || !strings.Contains(out.Body, "Привет!") || len(out.Attachments) != 1 || out.Attachments[0].Data != in.Attachments[0].Data || out.Attachments[0].Name != in.Attachments[0].Name {
		t.Fatalf("MIME roundtrip failed: %+v", out)
	}
}

package integrations

import (
	"context"
	"encoding/json"
	"io"
	"leadhelper/internal/domain"
	"net/http"
	"strings"
	"testing"
)

type graphCapture func(*http.Request) (*http.Response, error)

func (f graphCapture) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGraphRichPayload(t *testing.T) {
	g := Graph{Token: "fixture", AllowSend: true, Client: &http.Client{Transport: graphCapture(func(r *http.Request) (*http.Response, error) {
		var payload struct {
			Message struct {
				Body        struct{ ContentType, Content string }
				Attachments []struct {
					IsInline  bool
					ContentID string
				}
			}
		}
		if e := json.NewDecoder(r.Body).Decode(&payload); e != nil {
			t.Fatal(e)
		}
		if payload.Message.Body.ContentType != "HTML" || !strings.Contains(payload.Message.Body.Content, "cid:photo") || len(payload.Message.Attachments) != 1 || !payload.Message.Attachments[0].IsInline || payload.Message.Attachments[0].ContentID != "photo" {
			t.Fatal("lost rich message payload")
		}
		return &http.Response{StatusCode: 202, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	e := g.Send(context.Background(), domain.Message{From: "me@example.test", To: "peer@example.test", Body: "photo", HTML: `<img src="cid:photo">`, Attachments: []domain.Attachment{{Name: "photo.png", Data: "aW1hZ2U=", ContentType: "image/png", Inline: true, ContentID: "photo"}}})
	if e != nil {
		t.Fatal(e)
	}
}

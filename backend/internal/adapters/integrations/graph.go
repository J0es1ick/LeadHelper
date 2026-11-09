package integrations

import (
	"context"
	"fmt"
	"leadhelper/internal/domain"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

type Graph struct {
	Token     string
	AllowSend bool
	Client    *http.Client
}

func (g *Graph) Configured() bool     { return g.Token != "" }
func (g *Graph) SendConfigured() bool { return g.Configured() && g.AllowSend }

type recipient struct {
	EmailAddress struct {
		Address string `json:"address"`
		Name    string `json:"name"`
	} `json:"emailAddress"`
}

func addresses(rs []recipient) string {
	values := []string{}
	for _, r := range rs {
		values = append(values, r.EmailAddress.Address)
	}
	return strings.Join(values, ", ")
}
func (g *Graph) FetchMessages(ctx context.Context) ([]domain.Message, error) {
	c := g.Client
	if c == nil {
		c = client()
	}
	out := []domain.Message{}
	next := "https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages?$top=50&$orderby=receivedDateTime%20desc&$expand=attachments"
	for page := 0; page < 10; page++ {
		u, e := url.Parse(next)
		if e != nil || u.Scheme != "https" || u.Host != "graph.microsoft.com" {
			return nil, fmt.Errorf("Недопустимый адрес страницы Microsoft Graph")
		}
		var res struct {
			Next  string `json:"@odata.nextLink"`
			Value []struct {
				ID      string
				Subject string
				Body    struct {
					ContentType string
					Content     string
				}
				From             recipient
				ToRecipients     []recipient
				CcRecipients     []recipient
				ReceivedDateTime time.Time
				IsRead           bool
				Attachments      []struct {
					ID           string
					Name         string
					ContentType  string
					ContentBytes string
				}
			}
		}
		e = request(ctx, c, "GET", next, nil, map[string]string{"Authorization": "Bearer " + g.Token, "Prefer": "outlook.body-content-type=\"text\""}, &res)
		if e != nil {
			return nil, e
		}
		for _, v := range res.Value {
			m := domain.Message{ID: "graph-" + v.ID, ExternalID: v.ID, From: v.From.EmailAddress.Address, FromName: v.From.EmailAddress.Name, To: addresses(v.ToRecipients), CC: addresses(v.CcRecipients), Subject: v.Subject, Body: v.Body.Content, Folder: "inbox", Read: v.IsRead, Date: v.ReceivedDateTime, Attachments: []domain.Attachment{}}
			for _, a := range v.Attachments {
				if a.ContentBytes != "" {
					m.Attachments = append(m.Attachments, domain.Attachment{ID: a.ID, Name: a.Name, ContentType: a.ContentType, Data: a.ContentBytes})
				}
			}
			out = append(out, m)
		}
		if res.Next == "" {
			return out, nil
		}
		next = res.Next
	}
	return nil, fmt.Errorf("В ящике больше 500 писем: для первого подключения нужен более узкий период синхронизации")
}
func (g *Graph) ValidateSend(m domain.Message) error {
	if !g.AllowSend {
		return fmt.Errorf("Реальная отправка отключена. Установите ALLOW_LIVE_MAIL_SEND=true в .env")
	}
	if m.From == "" {
		return fmt.Errorf("Не указан отправитель")
	}
	size := 0
	for _, a := range m.Attachments {
		size += len(a.Data)
	}
	if size > 3*1024*1024 {
		return fmt.Errorf("Для отправки через Graph суммарный размер вложений в base64 должен быть до 3 МБ")
	}
	return nil
}
func (g *Graph) Send(ctx context.Context, m domain.Message) error {
	if e := g.ValidateSend(m); e != nil {
		return e
	}
	c := g.Client
	if c == nil {
		c = client()
	}
	to := func(value string) []any {
		out := []any{}
		rs, _ := mail.ParseAddressList(value)
		for _, r := range rs {
			out = append(out, map[string]any{"emailAddress": map[string]string{"address": r.Address}})
		}
		return out
	}
	attachments := []any{}
	size := 0
	for _, a := range m.Attachments {
		size += len(a.Data)
		attachments = append(attachments, map[string]any{"@odata.type": "#microsoft.graph.fileAttachment", "name": a.Name, "contentType": a.ContentType, "contentBytes": a.Data, "isInline": a.Inline, "contentId": a.ContentID})
	}
	if size > 3*1024*1024 {
		return fmt.Errorf("Для отправки через Graph суммарный размер вложений в base64 должен быть до 3 МБ")
	}
	kind, content := "Text", m.Body
	if m.HTML != "" {
		kind, content = "HTML", m.HTML
	}
	body := map[string]any{"message": map[string]any{"subject": m.Subject, "body": map[string]string{"contentType": kind, "content": content}, "toRecipients": to(m.To), "ccRecipients": to(m.CC), "attachments": attachments}, "saveToSentItems": true}
	return request(ctx, c, "POST", "https://graph.microsoft.com/v1.0/me/sendMail", body, map[string]string{"Authorization": "Bearer " + g.Token}, nil)
}

package integrations

import (
	"context"
	"fmt"
	"leadhelper/internal/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBitrixPhotosComeFromExistingResponses(t *testing.T) {
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch {
		case strings.Contains(r.URL.Path, "im.recent.list"):
			fmt.Fprint(w, `{"result":{"items":[{"id":42,"type":"chat","title":"Команда","avatar":{"url":"https://cdn-ru.bitrix24.ru/team.jpg"},"message":{"id":99,"text":"[USER=7 REPLACE]Анна[/USER] пригласила коллег"}},{"id":7,"type":"user","user":{"name":"Анна","avatar":"/upload/person.jpg"},"message":{}}]}}`)
		case strings.Contains(r.URL.Path, "im.dialog.messages.get"):
			fmt.Fprint(w, `{"result":{"messages":[{"id":99,"author_id":7,"text":"Привет"},{"id":100,"author_id":0,"text":"[USER=7 REPLACE]Анна[/USER] вошла"}],"users":[{"id":7,"name":"Анна","avatar":"https://cdn-ru.bitrix24.ru/person.jpg"}]}}`)
		case strings.Contains(r.URL.Path, "user.get"):
			fmt.Fprint(w, `{"result":[{"ID":"7","NAME":"Анна","LAST_NAME":"Иванова","ACTIVE":true,"PERSONAL_PHOTO":"/upload/person.jpg"}],"total":1}`)
		default:
			t.Error("unexpected extra request", r.URL.Path)
		}
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/test/", Client: srv.Client()}
	p, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "chats"})
	if e != nil || len(p.Items) != 2 {
		t.Fatal(p, e)
	}
	if p.Items[0].AvatarURL != "https://cdn-ru.bitrix24.ru/team.jpg" || p.Items[1].AvatarURL != srv.URL+"/upload/person.jpg" || strings.Contains(p.Items[0].Text, "[USER") {
		t.Fatal(p.Items)
	}
	p, e = b.Browse(context.Background(), domain.RemoteQuery{Kind: "messages", ID: "chat42"})
	if e != nil || len(p.Items) != 2 {
		t.Fatal(p, e)
	}
	for _, m := range p.Items {
		if m.ID == "99" && m.AvatarURL != "https://cdn-ru.bitrix24.ru/person.jpg" {
			t.Fatal(m)
		}
		if m.ID == "100" && (!m.System || m.AvatarURL != "") {
			t.Fatal(m)
		}
	}
	p, e = b.Browse(context.Background(), domain.RemoteQuery{Kind: "people"})
	if e != nil || len(p.Items) != 1 || p.Items[0].AvatarURL != srv.URL+"/upload/person.jpg" || calls != 3 {
		t.Fatal(p, e, calls)
	}
}

func TestBitrixAvatarURLRejectsCredentialsAndUntrustedOrigins(t *testing.T) {
	b := &Bitrix{Webhook: "https://portal.bitrix24.ru/rest/7/secret/"}
	for _, raw := range []string{"0", "", "javascript:alert(1)", "data:image/svg+xml,test", "http://portal.bitrix24.ru/a.jpg", "https://evil.test/a.jpg", "/rest/7/secret/avatar", "/%72est/7/secret/avatar", "/a.jpg?auth=secret", "https://user:pass@portal.bitrix24.ru/a.jpg", "https://portal.bitrix24.ru.evil.test/a.jpg"} {
		if got := b.avatarURL(raw); got != "" {
			t.Fatalf("accepted %q", raw)
		}
	}
	if got := b.avatarURL(false, map[string]any{"url": "/upload/avatar.jpg"}); got != "https://portal.bitrix24.ru/upload/avatar.jpg" {
		t.Fatal(got)
	}
}

package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"leadhelper/internal/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBitrixReadExactMessageAndRejectedAcknowledgement(t *testing.T) {
	response := `{"result":{"dialogId":"chat42","lastId":101,"counter":1}}`
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !strings.HasSuffix(r.URL.Path, "/im.dialog.read.json") || body["DIALOG_ID"] != "chat42" || body["MESSAGE_ID"] != float64(101) {
			t.Error("unbounded or wrong read request", body)
		}
		fmt.Fprint(w, response)
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/test/", Client: srv.Client()}
	ctx := context.Background()
	got, e := b.ReadChat(ctx, "chat42", "101")
	if e != nil || got.Unread != 1 || got.MessageID != "101" {
		t.Fatal(got, e)
	}
	for _, invalid := range []string{"", "0", "-1", "bad"} {
		if _, e = b.ReadChat(ctx, "chat42", invalid); e == nil {
			t.Fatal("accepted invalid read ID")
		}
	}
	if calls != 1 {
		t.Fatal("invalid requests reached Bitrix")
	}
	for _, bad := range []string{`{"result":false}`, `{"result":{"dialogId":"chat42","lastId":100,"counter":0}}`, `{"result":{"dialogId":"chat99","lastId":101,"counter":0}}`, `{"error":"ACCESS_DENIED"}`} {
		response = bad
		if _, e = b.ReadChat(ctx, "chat42", "101"); e == nil {
			t.Fatal("unconfirmed read accepted", bad)
		}
	}
}
func TestBitrixRecentFileOnlyMessageAndReadCursor(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"items":[{"id":42,"type":"chat","title":"Файлы","counter":1,"last_id":100,"message":{"id":101,"text":"","file":true}},{"id":43,"type":"user","message":{"id":102,"sticker":7}},{"id":44,"type":"user","message":{"id":103,"text":"Подпись","file":true}},{"id":45,"type":"user","message":{}}]}}`)
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/test/", Client: srv.Client()}
	p, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "chats"})
	if e != nil {
		t.Fatal(e)
	}
	if p.AccountID != b.AccountID() || p.Items[0].Text != "📎 Вложение" || p.Items[0].MessageID != "101" || p.Items[0].ReadMessageID != "100" || p.Items[0].Unread != 1 || p.Items[1].Text != "Стикер" || p.Items[2].Text != "Подпись" || p.Items[3].Text != "" {
		t.Fatal(p)
	}
}

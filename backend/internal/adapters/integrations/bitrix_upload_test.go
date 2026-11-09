package integrations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"leadhelper/internal/domain"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBitrixUploadPublishesOneFileAndCaptionWithoutRetry(t *testing.T) {
	calls := 0
	fail := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/rest/7/test/im.v2.File.upload.json" {
			t.Fatal(r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		fields := obj(body["fields"])
		data, e := base64.StdEncoding.DecodeString(str(fields["content"]))
		if e != nil || string(data) != "test bytes" || fields["name"] != "отчёт.txt" || fields["message"] != "Готово" || body["dialogId"] != "chat42" {
			t.Error("wrong upload payload")
		}
		if fail {
			fmt.Fprint(w, `{"result":{"messageId":0,"file":{"id":11}}}`)
			return
		}
		fmt.Fprint(w, `{"result":{"messageId":99,"dialogId":"chat42","file":{"id":11}}}`)
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/test/", Client: srv.Client()}
	in := domain.ChatUpload{Name: "отчёт.txt", Text: "Готово", Data: []byte("test bytes")}
	result, e := b.SendChatFile(context.Background(), "chat42", in)
	if e != nil || result.MessageID != "99" || result.FileID != "11" || calls != 1 {
		t.Fatal(result, e, calls)
	}
	fail = true
	_, e = b.SendChatFile(context.Background(), "chat42", in)
	if e == nil || calls != 2 {
		t.Fatal("unconfirmed send was accepted or retried", e, calls)
	}
	_, e = b.SendChatFile(context.Background(), "bad", in)
	if e == nil || calls != 2 {
		t.Fatal("invalid dialog sent")
	}
}

package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"leadhelper/internal/domain"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBitrixCardsAndReferences(t *testing.T) {
	b := &Bitrix{Webhook: "https://portal.bitrix24.ru/rest/7/secret/"}
	var m map[string]any
	if e := json.Unmarshal([]byte(`{"params":{"ATTACH":[{"DESCRIPTION":"Анализ звонка","BLOCKS":[{"MESSAGE":"Выводы"},{"GRID":[{"NAME":"Решение","VALUE":"Согласовать"}]},{"LINK":[{"NAME":"План","LINK":"/plan/"},{"NAME":"Bad","LINK":"javascript:alert(1)"}]}]}]}}`), &m); e != nil {
		t.Fatal(e)
	}
	cards := b.messageCards(m)
	if len(cards) != 1 || cards[0].Blocks[0].Text != "Выводы" || cards[0].Blocks[1].Text != "Согласовать" || cards[0].Blocks[2].URL != "https://portal.bitrix24.ru/plan/" || cards[0].Blocks[3].URL != "" {
		t.Fatal(cards)
	}
	refs := b.messageReferences("Звонок (/online/?IM_DIALOG=chat42&IM_MESSAGE=99) https://evil.test/online/?IM_DIALOG=chat42&IM_MESSAGE=100")
	if len(refs) != 1 || refs[0].MessageID != "99" {
		t.Fatal(refs)
	}
}
func TestBitrixLinkedMessageFetchIsBoundedAndReadOnly(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var args map[string]any
		_ = json.NewDecoder(r.Body).Decode(&args)
		if r.URL.Path != "/rest/7/test/im.dialog.messages.get.json" || args["LAST_ID"] != float64(100) || args["LIMIT"] != float64(1) {
			t.Error(r.URL.Path, args)
		}
		fmt.Fprint(w, `{"result":{"messages":[{"id":99,"author_id":0,"text":"BitrixGPT результат","params":{"ATTACH":[{"BLOCKS":[{"MESSAGE":"Анализ"}]}]}}]}}`)
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/test/", Client: srv.Client()}
	out, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "message", ID: "chat42", Cursor: "99"})
	if e != nil || len(out.Items) != 1 || out.Items[0].Title != "BitrixGPT" || len(out.Items[0].Cards) != 1 {
		t.Fatal(out, e)
	}
}

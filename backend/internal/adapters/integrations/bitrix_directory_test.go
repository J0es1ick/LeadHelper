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

func TestBitrixEmployeeFiltersAndTotals(t *testing.T) {
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var q map[string]any
		json.NewDecoder(r.Body).Decode(&q)
		filter := obj(q["FILTER"])
		if filter["USER_TYPE"] != "employee" {
			t.Error("not restricted to employee accounts")
		}
		if calls == 1 {
			if filter["ACTIVE"] != false || filter["NAME_SEARCH"] != "Анна" || filter["UF_DEPARTMENT"] != float64(12) || q["start"] != float64(50) {
				t.Error("lost dismissal/search/department/page filter", q)
			}
			fmt.Fprint(w, `{"result":[{"ID":"7","NAME":"Анна","LAST_NAME":"Иванова","ACTIVE":false,"UF_DEPARTMENT":[12,"13"],"IS_ONLINE":"N"}],"total":73,"next":100}`)
		} else {
			if len(filter) != 1 || q["start"] != float64(0) {
				t.Error("overall count restricted by filter")
			}
			fmt.Fprint(w, `{"result":[],"total":246}`)
		}
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/secret/", Client: srv.Client()}
	p, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "people", Status: "dismissed", Search: "Анна", Department: "12", Cursor: "50"})
	if e != nil {
		t.Fatal(e)
	}
	if calls != 2 || p.Next != "100" || p.Total == nil || *p.Total != 73 || p.TotalAll == nil || *p.TotalAll != 246 || len(p.Items) != 1 || p.Items[0].Status != "dismissed" || strings.Join(p.Items[0].Departments, ",") != "12,13" {
		t.Fatalf("wrong employee page: %+v", p)
	}
}
func TestBitrixWholeStructureAndLeaderNames(t *testing.T) {
	deptCalls := 0
	headCalls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		json.NewDecoder(r.Body).Decode(&q)
		if strings.Contains(r.URL.Path, "department.get") {
			deptCalls++
			if deptCalls == 1 {
				fmt.Fprint(w, `{"result":[{"ID":"1","NAME":"Компания","PARENT":"0","UF_HEAD":"7","SORT":100},{"ID":"2","NAME":"Разработка","PARENT":"1","UF_HEAD":"7","SORT":200}],"next":50}`)
			} else {
				if q["start"] != float64(50) {
					t.Error("missing department page")
				}
				fmt.Fprint(w, `{"result":[{"ID":"3","NAME":"Дизайн","PARENT":"2","UF_HEAD":"0","SORT":300}]}`)
			}
		} else if strings.Contains(r.URL.Path, "user.get") {
			headCalls++
			ids := obj(q["FILTER"])["ID"].([]any)
			if len(ids) != 1 || ids[0] != "7" {
				t.Error("leaders not deduplicated", ids)
			}
			fmt.Fprint(w, `{"result":[{"ID":"7","NAME":"Иван","LAST_NAME":"Петров","ACTIVE":false}]}`)
		} else {
			t.Error("unexpected method")
		}
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/secret/", Client: srv.Client()}
	p, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "departments"})
	if e != nil {
		t.Fatal(e)
	}
	if deptCalls != 2 || headCalls != 1 || len(p.Items) != 3 || p.Total == nil || *p.Total != 3 || p.Items[1].Parent != "1" || p.Items[1].HeadName != "Иван Петров" || !strings.HasSuffix(p.Items[0].HeadURL, "/user/7/") || p.Items[2].HeadID != "" {
		t.Fatalf("incomplete hierarchy: %+v", p)
	}
}
func TestBitrixNewsUsesIDCursorAndNewestFirst(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		json.NewDecoder(r.Body).Decode(&q)
		if strings.Contains(r.URL.Path, "user.get") {
			fmt.Fprint(w, `{"result":[{"ID":"7","NAME":"Анна","LAST_NAME":"Иванова"}]}`)
			return
		}
		if q["LIMIT"] != float64(30) {
			t.Error("unbounded news page")
		}
		last := int(q["LAST_ID"].(float64))
		if last == 0 {
			rows := map[string]any{"next": 305}
			for i := 0; i < 30; i++ {
				rows[fmt.Sprint(i)] = map[string]any{"ID": fmt.Sprint(100 - i), "AUTHOR_ID": "7", "TITLE": "News", "DATE_PUBLISH": fmt.Sprintf("2026-11-%02dT10:00:00+03:00", 30-i)}
			}
			json.NewEncoder(w).Encode(map[string]any{"result": rows})
		} else {
			if last != 305 {
				t.Error("offset used instead of last news ID", last)
			}
			fmt.Fprint(w, `{"result":[{"ID":"70","AUTHOR_ID":"7","TITLE":"Earlier","DATE_PUBLISH":"2026-10-30T10:00:00+03:00"}]}`)
		}
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/secret/", Client: srv.Client()}
	p, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "news"})
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 30 || p.Items[0].ID != "100" || p.Next != "305" || p.Items[0].Author != "Анна Иванова" {
		t.Fatalf("wrong news order/paging: %+v", p)
	}
	next, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "news", Cursor: p.Next})
	if e != nil || len(next.Items) != 1 || next.Next != "" || next.Items[0].ID != "70" {
		t.Fatal("wrong next news page", next, e)
	}
}
func TestBitrixNewsRejectsRepeatedPage(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"0":{"ID":"100"},"next":305}}`)
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/secret/", Client: srv.Client()}
	if _, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "news", Cursor: "305"}); e == nil {
		t.Fatal("repeated page accepted")
	}
}

// Feed pages can be empty or sparse when records refer to inaccessible/non-post events.
func TestBitrixNewsKeepsCursorOnEmptyAndShortPages(t *testing.T) {
	for _, payload := range []string{`{"result":{"next":200}}`, `{"result":{"0":{"ID":"900","TITLE":"Post"},"next":200}}`} {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, payload) }))
		b := &Bitrix{Webhook: srv.URL + "/rest/7/secret/", Client: srv.Client()}
		p, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "news", Cursor: "305"})
		if e != nil || p.Next != "200" {
			t.Fatal("short feed page lost continuation", p.Next, e)
		}
		srv.Close()
	}
}

func TestBitrixNewsAcceptsTopLevelFeedCursor(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":[{"ID":"978","TITLE":"New"}],"next":496272}`)
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/secret/", Client: srv.Client()}
	p, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "news"})
	if e != nil || len(p.Items) != 1 || p.Next != "496272" {
		t.Fatal("provider feed cursor lost", p.Next, e)
	}
}

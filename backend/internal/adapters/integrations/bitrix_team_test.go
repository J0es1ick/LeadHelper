package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"leadhelper/internal/domain"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

func TestManagedTeamRootsAndAllDescendants(t *testing.T) {
	rows := []domain.RemoteItem{{ID: "1", Title: "Компания", HeadID: "1"}, {ID: "2", Title: "Разработка", Parent: "1", HeadID: "7"}, {ID: "3", Title: "Frontend", Parent: "2", HeadID: "8"}, {ID: "4", Title: "Mobile", Parent: "3", HeadID: "7"}, {ID: "5", Title: "Продажи", Parent: "1", HeadID: "9"}}
	roots, depts, e := managedDepartments(rows, "7")
	if e != nil || len(roots) != 1 || roots[0].ID != "2" || len(depts) != 3 {
		t.Fatal("wrong management closure", roots, depts, e)
	}
	if _, _, e := managedDepartments(rows, "50"); e == nil {
		t.Fatal("employee without managed department was accepted")
	}
	if _, _, e := managedDepartments([]domain.RemoteItem{{ID: "1", Parent: "2", HeadID: "7"}, {ID: "2", Parent: "1"}}, "7"); e == nil {
		t.Fatal("cyclic hierarchy accepted")
	}
}
func TestBitrixTeamPagesDeduplicatesAndExcludesManager(t *testing.T) {
	pages := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		json.NewDecoder(r.Body).Decode(&q)
		switch {
		case strings.Contains(r.URL.Path, "profile"):
			fmt.Fprint(w, `{"result":{"ID":"7","NAME":"Юрий","LAST_NAME":"Руководитель"}}`)
		case strings.Contains(r.URL.Path, "department.get"):
			fmt.Fprint(w, `{"result":[{"ID":"1","NAME":"Компания","UF_HEAD":"1"},{"ID":"2","NAME":"Разработка","PARENT":"1","UF_HEAD":"7"},{"ID":"3","NAME":"Frontend","PARENT":"2","UF_HEAD":"8"},{"ID":"4","NAME":"Mobile","PARENT":"3","UF_HEAD":"8"},{"ID":"5","NAME":"Продажи","PARENT":"1","UF_HEAD":"9"}]}`)
		case strings.Contains(r.URL.Path, "user.get"):
			f := obj(q["FILTER"])
			if f["ID"] != nil {
				fmt.Fprint(w, `{"result":[]}`)
				return
			}
			if f["ACTIVE"] != true || f["USER_TYPE"] != "employee" {
				t.Error("inactive/external accounts not filtered")
			}
			ids := bitrixIDs(f["UF_DEPARTMENT"])
			sort.Strings(ids)
			if strings.Join(ids, ",") != "2,3,4" {
				t.Error("missing nested department filter", ids)
			}
			pages++
			if pages == 1 {
				fmt.Fprint(w, `{"result":[{"ID":"7","NAME":"Manager","ACTIVE":true,"UF_DEPARTMENT":[2]},{"ID":"8","NAME":"Team","ACTIVE":true,"UF_DEPARTMENT":[3,4]},{"ID":"9","NAME":"Dismissed","ACTIVE":false,"UF_DEPARTMENT":[3]},{"ID":"10","NAME":"Other","ACTIVE":true,"UF_DEPARTMENT":[5]}],"next":50}`)
			} else {
				if q["start"] != float64(50) {
					t.Error("pagination lost")
				}
				fmt.Fprint(w, `{"result":[{"ID":"8","NAME":"Team","ACTIVE":true,"UF_DEPARTMENT":[3,4]},{"ID":"11","NAME":"Nested","ACTIVE":true,"UF_DEPARTMENT":[4]}]}`)
			}
		default:
			t.Error("unexpected method")
		}
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/secret/", Client: srv.Client()}
	snapshot, e := b.FetchTeam(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if pages != 2 || len(snapshot.Members) != 2 || snapshot.ManagerID != "7" || len(snapshot.Roots) != 1 || len(snapshot.Departments) != 3 {
		t.Fatal("wrong team", snapshot)
	}
	for _, m := range snapshot.Members {
		if m.ID != "8" && m.ID != "11" {
			t.Fatal("not a subordinate", m.ID)
		}
	}
}
func TestBitrixTeamFailureOnSecondPage(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		json.NewDecoder(r.Body).Decode(&q)
		switch {
		case strings.Contains(r.URL.Path, "profile"):
			fmt.Fprint(w, `{"result":{"ID":"7"}}`)
		case strings.Contains(r.URL.Path, "department.get"):
			fmt.Fprint(w, `{"result":[{"ID":"2","NAME":"Team","UF_HEAD":"7"}]}`)
		case obj(q["FILTER"])["ID"] != nil:
			fmt.Fprint(w, `{"result":[]}`)
		case q["start"] == float64(0):
			fmt.Fprint(w, `{"result":[{"ID":"8","NAME":"User","ACTIVE":true,"UF_DEPARTMENT":[2]}],"next":50}`)
		default:
			fmt.Fprint(w, `{"error":"ACCESS_DENIED"}`)
		}
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/secret/", Client: srv.Client()}
	if _, e := b.FetchTeam(context.Background()); e == nil {
		t.Fatal("partial roster accepted")
	}
}

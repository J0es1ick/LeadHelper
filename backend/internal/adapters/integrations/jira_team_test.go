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

func TestTeamJQLAlwaysConstrainsAssignees(t *testing.T) {
	for _, tc := range []struct{ extra, want string }{
		{"", `assignee IN ("anna", "max") ORDER BY updated DESC, key ASC`},
		{`project = ONE OR project = TWO ORDER BY priority DESC`, `assignee IN ("anna", "max") AND (project = ONE OR project = TWO) ORDER BY priority DESC`},
		{`summary ~ "ORDER BY" AND (status = Open OR status = Review)`, `assignee IN ("anna", "max") AND (summary ~ "ORDER BY" AND (status = Open OR status = Review)) ORDER BY updated DESC, key ASC`},
		{"order\tby created DESC", "assignee IN (\"anna\", \"max\") order\tby created DESC"},
	} {
		q, e := teamJQL(tc.extra, []string{"anna", "max", "anna"})
		if e != nil || q != tc.want {
			t.Fatalf("%q -> %q / %v", tc.extra, q, e)
		}
	}
	if _, e := teamJQL("", nil); e == nil {
		t.Fatal("empty team allowed global request")
	}
	if _, e := teamJQL(`project = "bad`, []string{"anna"}); e == nil {
		t.Fatal("broken quote allowed")
	}
	q, e := teamJQL("", []string{`a" OR assignee IS NOT EMPTY OR "x`})
	if e != nil || !strings.Contains(q, `a\" OR assignee IS NOT EMPTY OR \"x`) {
		t.Fatal("JQL identity not escaped", q, e)
	}
}
func TestJiraTeamFiltersOnServerAndMapsCanonicalMember(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		t.Run(fmt.Sprint(cloud), func(t *testing.T) {
			calls := 0
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				expected := `assignee IN ("anna") AND (project = ONE OR project = TWO)`
				if cloud {
					expected = `assignee IN ("account-a") AND (project = ONE OR project = TWO)`
				}
				if !strings.HasPrefix(body["jql"].(string), expected) {
					t.Error("unscoped request", body["jql"])
				}
				if calls == 1 {
					if cloud {
						fmt.Fprint(w, `{"nextPageToken":"next","issues":[{"key":"ONE-1","fields":{"assignee":{"accountId":"account-a"}}}]}`)
					} else {
						fmt.Fprint(w, `{"total":2,"issues":[{"key":"ONE-1","fields":{"assignee":{"key":"KEY-A","name":"anna"}}}]}`)
					}
				} else {
					if cloud {
						fmt.Fprint(w, `{"isLast":true,"issues":[{"key":"TWO-1","fields":{"assignee":{"accountId":"account-a"}}}]}`)
					} else {
						fmt.Fprint(w, `{"total":2,"issues":[{"key":"TWO-1","fields":{"assignee":{"key":"KEY-A","name":"anna"}}}]}`)
					}
				}
			}))
			defer srv.Close()
			j := &Jira{URL: srv.URL, Token: "secret", Email: "user", Client: srv.Client(), JQL: "project = ONE OR project = TWO"}
			m := domain.Member{ID: "bitrix-local-a", Source: "bitrix", JiraUserID: "KEY-A", JiraLogin: "anna"}
			if cloud {
				j.Mode = "cloud"
				m.JiraUserID = "account-a"
				m.JiraLogin = "account-a"
			}
			m.JiraSite = j.SiteID()
			tasks, e := j.FetchTeamTasks(context.Background(), []domain.Member{m})
			if e != nil || len(tasks) != 2 || calls != 2 {
				t.Fatal(tasks, e, calls)
			}
			for _, task := range tasks {
				if task.AssigneeID != m.ID {
					t.Fatal("duplicate member identity", task)
				}
			}
		})
	}
}
func TestJiraTeamRejectsOutsideAssigneeAndIncompleteMapping(t *testing.T) {
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"total":1,"issues":[{"key":"X-1","fields":{"assignee":{"key":"outsider"}}}]}`)
	}))
	defer srv.Close()
	j := &Jira{URL: srv.URL, Token: "secret", Client: srv.Client()}
	if _, _, e := j.FetchTasks(context.Background()); e == nil || calls != 0 {
		t.Fatal("default fetched all tasks")
	}
	if _, e := j.FetchTeamTasks(context.Background(), nil); e == nil || calls != 0 {
		t.Fatal("empty roster fetched tasks")
	}
	m := domain.Member{ID: "a", Source: "bitrix", JiraSite: j.SiteID(), JiraUserID: "k", JiraLogin: "anna"}
	if _, e := j.FetchTeamTasks(context.Background(), []domain.Member{m, {ID: "b"}}); e == nil || calls != 0 {
		t.Fatal("partial mapping fetched tasks")
	}
	if tasks, e := j.FetchTeamTasks(context.Background(), []domain.Member{m}); e == nil || tasks != nil {
		t.Fatal("foreign assignee accepted", tasks, e)
	}
}
func TestJiraUserSearchAndValidation(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/2/user/search" {
			if r.URL.Query().Get("username") != "anna@example.test" || r.URL.Query().Get("maxResults") != "50" {
				t.Error("wrong search", r.URL)
			}
			fmt.Fprint(w, `[{"key":"K1","name":"anna","displayName":"Anna","emailAddress":"anna@example.test","active":true},{"key":"old","name":"old","active":false}]`)
		} else {
			if r.URL.Query().Get("key") != "K1" {
				t.Error("wrong user query")
			}
			fmt.Fprint(w, `{"key":"K1","name":"anna","active":true}`)
		}
	}))
	defer srv.Close()
	j := &Jira{URL: srv.URL, Token: "secret", Client: srv.Client()}
	users, e := j.SearchUsers(context.Background(), "anna@example.test")
	if e != nil || len(users) != 1 || users[0].ID != "K1" {
		t.Fatal(users, e)
	}
	if u, e := j.User(context.Background(), "K1"); e != nil || u.Login != "anna" {
		t.Fatal(u, e)
	}
}

func TestJiraExcessiveTotalStopsOnFirstPage(t *testing.T) {
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"total":10001,"issues":[]}`)
	}))
	defer srv.Close()
	j := &Jira{URL: srv.URL, Token: "secret", Client: srv.Client()}
	m := domain.Member{ID: "a", Source: "bitrix", JiraSite: j.SiteID(), JiraUserID: "K", JiraLogin: "anna"}
	if tasks, e := j.FetchTeamTasks(context.Background(), []domain.Member{m}); e == nil || tasks != nil || calls != 1 {
		t.Fatal("oversized selection was paged or applied", tasks, e, calls)
	}
}

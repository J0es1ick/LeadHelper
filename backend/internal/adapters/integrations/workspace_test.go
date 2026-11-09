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

func TestJiraServerAllProjectsPagination(t *testing.T) {
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/rest/api/2/search" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("wrong Jira Server request")
		}
		var q map[string]any
		json.NewDecoder(r.Body).Decode(&q)
		if strings.Contains(q["jql"].(string), "project") {
			t.Error("projects unexpectedly restricted")
		}
		if calls == 1 {
			fmt.Fprint(w, `{"total":2,"issues":[{"key":"ONE-1","fields":{"summary":"First","assignee":{"key":"user1","displayName":"Name"}}}]}`)
		} else {
			if q["startAt"] != float64(1) {
				t.Error("server page cursor missing")
			}
			fmt.Fprint(w, `{"total":2,"issues":[{"key":"TWO-1","fields":{"summary":"Second"}}]}`)
		}
	}))
	defer srv.Close()
	j := &Jira{URL: srv.URL, Mode: "server", Scope: "jql", JQL: "assignee is not EMPTY", Token: "secret", Client: srv.Client()}
	tasks, members, e := j.FetchTasks(context.Background())
	if e != nil || len(tasks) != 2 || len(members) != 1 || members[0].ID != "jira-user1" {
		t.Fatal("Server pagination/mapping failed", e)
	}
}
func TestBitrixCalendarTimezoneRepeatsAndDeclined(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "profile"):
			fmt.Fprint(w, `{"result":{"ID":"7","TIME_ZONE":"Europe/Moscow"}}`)
		case strings.Contains(r.URL.Path, "accessibility"):
			fmt.Fprint(w, `{"result":{"7":[{"ID":"1","NAME":"Meeting","DATE_FROM":"07.11.2026 10:00:00","DATE_TO":"07.11.2026 11:00:00","TZ_FROM":"Europe/Moscow","TZ_TO":"Europe/Moscow","DATE_FROM_TS_UTC":"1","ACCESSIBILITY":"busy"},{"ID":"1","NAME":"Meeting repeated","DATE_FROM":"08.11.2026 10:00:00","DATE_TO":"08.11.2026 11:00:00","ACCESSIBILITY":"busy"},{"ID":"2","NAME":"Declined","DATE_FROM":"07.11.2026 12:00:00","DATE_TO":"07.11.2026 13:00:00"},{"ID":"3","NAME":"Day off","DATE_FROM":"11/07/2026 12:00:00 am","DATE_TO":"11/07/2026 11:59:59 pm","DT_SKIP_TIME":"Y","ACCESSIBILITY":"absent"}]}}`)
		case strings.Contains(r.URL.Path, "event.get"):
			fmt.Fprint(w, `{"result":[{"ID":"1","LOCATION":"Room","MEETING_STATUS":"Y"},{"ID":"2","MEETING_STATUS":"N"}]}`)
		default:
			t.Error("unexpected method")
		}
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/7/secret/", Client: srv.Client()}
	page, e := b.Calendar(context.Background(), "2026-11-07", "2026-11-09")
	if e != nil {
		t.Fatal(e)
	}
	if len(page.Events) != 3 {
		t.Fatalf("expected repeats and all day, got %d", len(page.Events))
	}
	var meeting, allDay domain.CalendarEvent
	for _, v := range page.Events {
		if v.AllDay {
			allDay = v
		} else if v.Title == "Meeting" {
			meeting = v
		}
	}
	if meeting.Start.UTC().Hour() != 7 || meeting.Location != "Room" || allDay.End.Sub(allDay.Start).Hours() != 24 {
		t.Fatal("timezone/all-day mapping is incorrect")
	}
}
func TestConfluenceSearchEscapingAndPagination(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing PAT")
		}
		q := r.URL.Query()
		if q.Get("start") != "25" || q.Get("limit") != "25" || !strings.Contains(q.Get("cql"), `text~"quote\" OR type=attachment"`) {
			t.Error("CQL not escaped or page missing")
		}
		fmt.Fprint(w, `{"results":[{"id":"42","title":"Doc","space":{"name":"Team"},"_links":{"webui":"/pages/viewpage.action?pageId=42"}}],"_links":{"next":"https://evil.example/secret"}}`)
	}))
	defer srv.Close()
	c := &Confluence{URL: srv.URL, Token: "secret", Client: srv.Client()}
	p, e := c.Browse(context.Background(), domain.RemoteQuery{Kind: "pages", Cursor: "25", Search: `quote" OR type=attachment`})
	if e != nil || len(p.Items) != 1 || p.Next != "26" || !strings.HasPrefix(p.Items[0].URL, srv.URL) {
		t.Fatal("invalid Confluence paging", e)
	}
}
func TestHubDoesNotExposeOrForwardSavedSecretsToNewOrigin(t *testing.T) {
	saved := false
	h, _ := NewHub(map[string]domain.IntegrationConfig{"jira": {URL: "https://pm.example.test", Token: "private-token"}, "bitrix": {URL: "https://portal.example.test/rest/7/private-key/"}}, nil, func([]byte) error { saved = true; return nil })
	if c := h.PublicConfig("jira"); c.Token != "" {
		t.Fatal("token exposed")
	}
	if c := h.PublicConfig("bitrix"); strings.Contains(c.URL, "private-key") || strings.Contains(c.URL, "/rest/") {
		t.Fatal("webhook exposed")
	}
	if e := h.Configure(context.Background(), "jira", domain.IntegrationConfig{URL: "https://other.example.test"}); e == nil || saved {
		t.Fatal("retained token forwarded to new origin")
	}
}

package integrations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJiraPaginationAndMapping(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Error("wrong API endpoint")
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing authentication")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if requests == 1 {
			w.Write([]byte(`{"nextPageToken":"next","issues":[{"key":"TEST-1","fields":{"summary":"Done","description":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Context"}]}]},"status":{"statusCategory":{"key":"done"}},"assignee":{"accountId":"anna","displayName":"Anna"},"priority":{"name":"High"},"project":{"name":"Team"},"timeoriginalestimate":7200,"created":"2026-11-02T12:00:00.000+0300","resolutiondate":"2026-11-07T12:00:00.000+0300"}}]}`))
		} else {
			if body["nextPageToken"] != "next" {
				t.Error("missing page token")
			}
			w.Write([]byte(`{"isLast":true,"issues":[]}`))
		}
	}))
	defer server.Close()
	j := &Jira{URL: server.URL, Email: "test", Token: "secret", JQL: "project=TEST", Scope: "jql", Mode: "cloud", Client: server.Client()}
	tasks, members, e := j.FetchTasks(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if requests != 2 || len(tasks) != 1 || len(members) != 1 || tasks[0].Estimate != 2 || tasks[0].CompletedAt == nil || tasks[0].Description != "Context\n" {
		t.Fatalf("mapping failed: %+v", tasks)
	}
}
func TestCredentialsNeverAppearInProviderErrors(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401); w.Write([]byte("TOKEN-secret")) }))
	defer server.Close()
	b := &Jira{URL: server.URL, Email: "test", Token: "TOKEN-secret", Scope: "jql", JQL: "assignee is not EMPTY", Client: server.Client()}
	_, _, e := b.FetchTasks(context.Background())
	if e == nil || e.Error() != "Сервис вернул HTTP 401. Проверьте доступ и настройки интеграции" {
		t.Fatalf("unsafe error: %v", e)
	}
}

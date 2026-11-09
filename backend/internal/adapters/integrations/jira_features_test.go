package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leadhelper/internal/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJiraCreateUsesPagedMetadataAndCloudDocuments(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		t.Run(fmt.Sprint(cloud), func(t *testing.T) {
			var posted map[string]any
			pages := 0
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "createmeta") {
					pages++
					key := "values"
					if cloud {
						key = "fields"
					}
					rows := []any{map[string]any{"fieldId": "summary", "name": "Summary", "required": true, "schema": map[string]string{"type": "string"}}}
					if r.URL.Query().Get("startAt") == "1" {
						rows = []any{map[string]any{"fieldId": "description", "name": "Description", "schema": map[string]string{"type": "string"}}, map[string]any{"fieldId": "customfield_123", "name": "Release", "schema": map[string]string{"type": "option"}, "allowedValues": []any{map[string]string{"id": "42", "value": "Release A"}}}}
					}
					json.NewEncoder(w).Encode(map[string]any{"total": 3, key: rows})
					return
				}
				if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/issue") {
					json.NewDecoder(r.Body).Decode(&posted)
					w.WriteHeader(201)
					fmt.Fprint(w, `{"key":"TEAM-42","id":"42"}`)
					return
				}
				if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/issue/TEAM-42") {
					fmt.Fprint(w, `{"fields":{"summary":"Created task","description":"Description","project":{"name":"Team"},"status":{"statusCategory":{"key":"indeterminate"}},"timeoriginalestimate":5400}}`)
					return
				}
				t.Error("unexpected request", r.Method, r.URL.Path)
				w.WriteHeader(404)
			}))
			defer srv.Close()
			j := &Jira{URL: srv.URL, Token: "secret", Email: "qa", Client: srv.Client(), Mode: "server"}
			if cloud {
				j.Mode = "cloud"
			}
			result, e := j.CreateIssue(context.Background(), domain.JiraCreateInput{Project: "100", IssueType: "1", Fields: map[string]any{"summary": "Created task", "description": "One\nTwo", "customfield_123": map[string]string{"id": "42"}}})
			if e != nil {
				t.Fatal(e)
			}
			if pages != 2 || result.Task.ExternalID != "TEAM-42" || result.Task.Estimate != 1.5 || result.Task.Status != "progress" {
				t.Fatal(pages, result)
			}
			fields := obj(posted["fields"])
			if obj(fields["customfield_123"])["id"] != "42" || obj(fields["project"])["id"] != "100" {
				t.Fatal(posted)
			}
			if cloud {
				if obj(fields["description"])["type"] != "doc" || adfText(fields["description"]) != "One\nTwo\n" {
					t.Fatal(fields["description"])
				}
			} else if fields["description"] != "One\nTwo" {
				t.Fatal(fields)
			}
		})
	}
}
func TestWorklogsFetchAllPagesAndUseAuthorStartedDate(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		t.Run(fmt.Sprint(cloud), func(t *testing.T) {
			searches, logs := 0, 0
			fail := false
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "search") {
					searches++
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					jql := str(body["jql"])
					if !strings.Contains(jql, "worklogAuthor in") || strings.Contains(jql, "assignee") || strings.Contains(jql, "project = WRONG") || !strings.Contains(jql, "2026-11-08") {
						t.Error("incorrect worklog scope", jql)
					}
					second := body["startAt"] == float64(1) || body["nextPageToken"] == "next"
					key := "OTHER-1"
					if second {
						key = "DONE-2"
					}
					response := map[string]any{"total": 2, "issues": []any{map[string]any{"key": key, "fields": map[string]string{"summary": "Work by author"}}}}
					if cloud {
						if second {
							response["isLast"] = true
						} else {
							response["nextPageToken"] = "next"
						}
					}
					json.NewEncoder(w).Encode(response)
					return
				}
				logs++
				if fail && strings.Contains(r.URL.Path, "DONE-2") {
					w.WriteHeader(503)
					return
				}
				author := map[string]any{"key": "KEY-A", "name": "anna", "accountId": "account-a"}
				if strings.Contains(r.URL.Path, "DONE-2") {
					json.NewEncoder(w).Encode(map[string]any{"total": 1, "worklogs": []any{map[string]any{"id": "4", "author": author, "started": "2026-11-09T23:30:00.000+0000", "timeSpentSeconds": 900, "comment": "Next Moscow day"}}})
					return
				}
				rows := []any{map[string]any{"id": "1", "author": author, "started": "2026-11-08T22:30:00.000+0000", "timeSpentSeconds": 3600, "comment": "Cross midnight"}}
				if r.URL.Query().Get("startAt") == "1" {
					rows = []any{map[string]any{"id": "2", "author": map[string]string{"key": "OTHER", "accountId": "other"}, "started": "2026-11-09T10:00:00.000+0300", "timeSpentSeconds": 7200}, map[string]any{"id": "3", "author": author, "started": "2026-11-08T20:00:00.000+0000", "timeSpentSeconds": 1800}}
				}
				json.NewEncoder(w).Encode(map[string]any{"total": 3, "worklogs": rows})
			}))
			defer srv.Close()
			j := &Jira{URL: srv.URL, Token: "secret", Email: "qa", Client: srv.Client(), JQL: "project = WRONG", Mode: "server"}
			m := domain.Member{ID: "local-a", JiraUserID: "KEY-A", JiraLogin: "anna"}
			if cloud {
				j.Mode = "cloud"
				m.JiraUserID = "account-a"
				m.JiraLogin = "account-a"
			}
			m.JiraSite = j.SiteID()
			result, e := j.FetchWorklogs(context.Background(), []domain.Member{m}, "2026-11-09", "2026-11-15")
			if e != nil {
				t.Fatal(e)
			}
			if len(result) != 2 || searches != 2 || logs != 3 || result[0].Date != "2026-11-09" || result[1].Date != "2026-11-10" || result[0].MemberID != "local-a" || result[0].Seconds != 3600 {
				t.Fatal(result, searches, logs)
			}
			fail = true
			if partial, e := j.FetchWorklogs(context.Background(), []domain.Member{m}, "2026-11-09", "2026-11-15"); e == nil || partial != nil {
				t.Fatal("partial worklogs must not replace a snapshot", partial, e)
			}
		})
	}
}
func TestBitrixChatFilesJoinAndSignedDownload(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/1/secret/im.dialog.messages.get.json":
			fmt.Fprint(w, `{"result":{"messages":[{"id":1,"author_id":2,"text":"Attached","params":{"FILE_ID":[12,13]},"date":"2026-11-09T10:00:00+03:00"}],"users":[{"id":2,"name":"Colleague"}],"files":[{"id":12,"type":"image","name":"photo.png","size":100,"urlDownload":"https://example.test/private?secret=1"},{"id":13,"type":"file","name":"brief.pdf","size":200},{"id":14,"name":"unrelated.doc"}]}}`)
		case "/rest/1/secret/disk.file.get.json":
			json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"NAME": "photo.png", "DOWNLOAD_URL": srv.URL + "/download?signature=private"}})
		case "/rest/1/secret/disk.folder.getchildren.json":
			fmt.Fprint(w, `{"result":[{"ID":"13","NAME":"brief.pdf","SIZE":"200","TYPE":"file"},{"ID":"14","NAME":"Folder","TYPE":"folder"}]}`)
		case "/download":
			if r.Header.Get("Authorization") != "" {
				t.Error("credentials forwarded to download")
			}
			fmt.Fprint(w, "image bytes")
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	b := &Bitrix{Webhook: srv.URL + "/rest/1/secret/", Client: srv.Client()}
	p, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "messages", ID: "chat1"})
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || len(p.Items[0].Attachments) != 2 || !p.Items[0].Attachments[0].Image {
		t.Fatal(p)
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "/secret/") {
		t.Fatal("signed link exposed")
	}
	files, e := b.Browse(context.Background(), domain.RemoteQuery{Kind: "files", ID: "1"})
	if e != nil || len(files.Items) != 2 || len(files.Items[0].Attachments) != 1 || len(files.Items[1].Attachments) != 0 {
		t.Fatal("file browser preview metadata", files, e)
	}
	if file := files.Items[0].Attachments[0]; file.ID != "13" || file.Name != "brief.pdf" || file.Size != 200 || !strings.HasPrefix(file.URL, "/api/bitrix/files/13?account=") {
		t.Fatal(file)
	}
	body, name, _, _, e := b.DownloadFile(context.Background(), "12")
	if e != nil {
		t.Fatal(e)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if name != "photo.png" || string(data) != "image bytes" {
		t.Fatal(name, string(data))
	}
	for _, url := range []string{"http://127.0.0.1/file", "https://untrusted.example/file", "https://bitrix24.ru.evil.example/file", "https://user:secret@cdn.bitrix24.ru/file"} {
		if _, e = b.downloadURL(url); e == nil {
			t.Fatal("unsafe download allowed", url)
		}
	}
}

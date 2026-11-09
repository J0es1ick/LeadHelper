package integrations

import (
	"context"
	"encoding/base64"
	"fmt"
	"leadhelper/internal/domain"
	"net/http"
	"strings"
	"time"
)

type Jira struct {
	URL, Email, Token, JQL, Mode, Scope string
	Client                              *http.Client
}

func (j *Jira) Configured() bool {
	return j.URL != "" && j.Token != "" && (!j.cloud() || j.Email != "")
}
func (j *Jira) cloud() bool {
	return j.Mode == "cloud" || (j.Mode == "" && strings.Contains(j.URL, ".atlassian.net"))
}
func (j *Jira) FetchTasks(ctx context.Context) ([]domain.Task, []domain.Member, error) {
	if j.TeamScoped() {
		return nil, nil, fmt.Errorf("Сначала сопоставьте команду с пользователями Jira")
	}
	if strings.TrimSpace(j.JQL) == "" {
		return nil, nil, fmt.Errorf("Укажите JQL или выберите задачи команды")
	}
	return j.fetchTasks(ctx, j.JQL, nil)
}
func (j *Jira) fetchTasks(ctx context.Context, jql string, assignees map[string]string) ([]domain.Task, []domain.Member, error) {
	if e := httpsURL(j.URL); e != nil {
		return nil, nil, e
	}
	c := j.Client
	if c == nil {
		c = client()
	}
	tasks := []domain.Task{}
	members := []domain.Member{}
	seen := map[string]bool{}
	seenTasks := map[string]bool{}
	token := ""
	start := 0
	for page := 0; page < 100; page++ {
		body := map[string]any{"jql": jql, "maxResults": 100, "fields": []string{"summary", "description", "status", "assignee", "priority", "project", "duedate", "timeoriginalestimate", "created", "updated", "resolutiondate"}}
		path, auth := "/rest/api/2/search", "Bearer "+j.Token
		if j.cloud() {
			path = "/rest/api/3/search/jql"
			auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(j.Email+":"+j.Token))
		} else {
			body["startAt"] = start
		}
		if token != "" {
			body["nextPageToken"] = token
		}
		var res struct {
			Total         int
			NextPageToken string `json:"nextPageToken"`
			IsLast        bool   `json:"isLast"`
			Issues        []struct {
				Key    string
				Fields struct {
					Summary     string
					Description any
					Status      struct {
						Name           string
						StatusCategory struct{ Key string }
					}
					Assignee *struct {
						AccountID   string
						Key, Name   string
						DisplayName string
					}
					Priority             struct{ Name string }
					Project              struct{ Name string }
					DueDate              string
					TimeOriginalEstimate float64
					Created              string
					Updated              string
					ResolutionDate       string
				}
			}
		}
		e := request(ctx, c, "POST", strings.TrimRight(j.URL, "/")+path, body, map[string]string{"Authorization": auth}, &res)
		if e != nil {
			return nil, nil, e
		}
		if !j.cloud() && res.Total > 10000 {
			return nil, nil, fmt.Errorf("В выбранной команде или фильтре больше 10 000 задач. Уточните дополнительный JQL, например период; предыдущая загрузка сохранена")
		}
		for _, issue := range res.Issues {
			if issue.Key == "" || seenTasks[issue.Key] {
				return nil, nil, fmt.Errorf("Jira вернула повторную или некорректную страницу задач; предыдущая загрузка сохранена")
			}
			seenTasks[issue.Key] = true
			f := issue.Fields
			status := "todo"
			switch f.Status.StatusCategory.Key {
			case "done":
				status = "done"
			case "indeterminate":
				status = "progress"
			}
			priority := "medium"
			switch strings.ToLower(f.Priority.Name) {
			case "highest", "critical", "blocker", "критический", "блокирующий", "самый высокий":
				priority = "critical"
			case "high", "высокий":
				priority = "high"
			case "low", "lowest", "низкий", "самый низкий":
				priority = "low"
			}
			t := domain.Task{ID: "jira-" + issue.Key, ExternalID: issue.Key, Title: f.Summary, Description: adfText(f.Description), Status: status, Priority: priority, Source: "jira", Project: f.Project.Name, Estimate: f.TimeOriginalEstimate / 3600, DueDate: date(f.DueDate), URL: strings.TrimRight(j.URL, "/") + "/browse/" + issue.Key, Tags: []string{}, UpdatedAt: time.Now().UTC()}
			t.CreatedAt = providerTime(f.Created)
			if t.CreatedAt.IsZero() {
				t.CreatedAt = t.UpdatedAt
			}
			if status == "done" {
				v := providerTime(f.ResolutionDate)
				if !v.IsZero() {
					t.CompletedAt = &v
				}
			}
			if f.Assignee != nil {
				identity := f.Assignee.AccountID
				if identity == "" {
					identity = f.Assignee.Key
				}
				if identity == "" {
					identity = f.Assignee.Name
				}
				t.AssigneeID = "jira-" + identity
				if !seen[t.AssigneeID] {
					login := f.Assignee.Name
					if j.cloud() {
						login = f.Assignee.AccountID
					}
					members = append(members, domain.Member{ID: t.AssigneeID, Source: "jira", JiraUserID: identity, JiraLogin: login, JiraSite: j.SiteID(), Name: f.Assignee.DisplayName, Role: "Jira", Capacity: 40, Color: "#6591b0"})
					seen[t.AssigneeID] = true
				}
			}
			if assignees != nil {
				id, ok := assignees[t.AssigneeID]
				if !ok {
					return nil, nil, fmt.Errorf("Jira вернула задачу вне выбранной команды; предыдущая загрузка сохранена")
				}
				t.AssigneeID = id
			}
			tasks = append(tasks, t)
		}
		if !j.cloud() {
			start += len(res.Issues)
			if start >= res.Total {
				return tasks, members, nil
			}
			if len(res.Issues) == 0 {
				return nil, nil, fmt.Errorf("Jira вернула пустую страницу до конца списка")
			}
			continue
		}
		if res.IsLast || res.NextPageToken == "" {
			return tasks, members, nil
		}
		if res.NextPageToken == token {
			return nil, nil, fmt.Errorf("Jira вернула повторный маркер страницы")
		}
		token = res.NextPageToken
	}
	return nil, nil, fmt.Errorf("В выбранной команде или фильтре больше 10 000 задач. Уточните дополнительный JQL, например период; предыдущая загрузка сохранена")
}
func adfText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		out := ""
		if t, ok := x["text"].(string); ok {
			out = t
		}
		if arr, ok := x["content"].([]any); ok {
			for _, n := range arr {
				out += adfText(n)
			}
		}
		if x["type"] == "paragraph" || x["type"] == "heading" {
			out += "\n"
		}
		return out
	}
	return ""
}

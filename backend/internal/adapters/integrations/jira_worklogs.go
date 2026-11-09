package integrations

import (
	"context"
	"fmt"
	"leadhelper/internal/domain"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (j *Jira) FetchWorklogs(ctx context.Context, members []domain.Member, from, to string) ([]domain.Worklog, error) {
	out := []domain.Worklog{}
	aliases := map[string]string{}
	authors := []string{}
	for _, m := range members {
		if m.Inactive || m.Demo || m.JiraSite != j.SiteID() || m.JiraUserID == "" || m.JiraLogin == "" {
			continue
		}
		for _, a := range []string{m.JiraUserID, m.JiraLogin} {
			if old := aliases[a]; old != "" && old != m.ID {
				return nil, fmt.Errorf("Один пользователь Jira сопоставлен нескольким сотрудникам")
			}
			aliases[a] = m.ID
		}
		login := m.JiraLogin
		if j.cloud() {
			login = m.JiraUserID
		}
		authors = append(authors, `"`+strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(login)+`"`)
	}
	if len(authors) == 0 {
		return nil, fmt.Errorf("Сначала сопоставьте участников команды с Jira в интеграциях")
	}
	loc, _ := time.LoadLocation("Europe/Moscow")
	first, e := time.ParseInLocation("2006-01-02", from, loc)
	if e != nil {
		return nil, e
	}
	last, e := time.ParseInLocation("2006-01-02", to, loc)
	if e != nil || last.Before(first) || last.Sub(first) > 30*24*time.Hour {
		return nil, fmt.Errorf("Выберите не более 31 дня")
	}
	// Jira evaluates JQL dates in the querying user's timezone. Widen the search and
	// then filter every individual worklog in the workspace timezone, including work
	// on completed issues or issues currently assigned to somebody else.
	jql := "worklogAuthor in (" + strings.Join(authors, ",") + ") AND worklogDate >= \"" + first.AddDate(0, 0, -1).Format("2006-01-02") + "\" AND worklogDate <= \"" + last.AddDate(0, 0, 1).Format("2006-01-02") + "\" ORDER BY key ASC"
	seenIssues := map[string]bool{}
	seenLogs := map[string]bool{}
	token := ""
	start := 0
	for page := 0; page < 100; page++ {
		body := map[string]any{"jql": jql, "maxResults": 100, "fields": []string{"summary"}}
		path := "search"
		if j.cloud() {
			path = "search/jql"
			if token != "" {
				body["nextPageToken"] = token
			}
		} else {
			body["startAt"] = start
		}
		var res struct {
			Total         int
			NextPageToken string
			IsLast        bool
			Issues        []struct {
				Key    string
				Fields struct{ Summary string }
			}
		}
		if e = j.jsonRequest(ctx, "POST", path, body, &res); e != nil {
			return nil, e
		}
		if res.Total > 10000 {
			return nil, fmt.Errorf("Слишком много задач со списаниями; сократите период")
		}
		for _, issue := range res.Issues {
			if issue.Key == "" || seenIssues[issue.Key] {
				return nil, fmt.Errorf("Jira вернула повторную страницу задач со списаниями")
			}
			seenIssues[issue.Key] = true
			complete := false
			for offset := 0; offset < 100000; {
				var logs struct {
					Total    int
					Worklogs []struct {
						ID               string
						Started          string
						TimeSpentSeconds int64
						Comment          any
						Author           jiraUserRecord
					}
				}
				if e = j.jsonRequest(ctx, "GET", "issue/"+url.PathEscape(issue.Key)+"/worklog?maxResults=100&startAt="+strconv.Itoa(offset), nil, &logs); e != nil {
					return nil, e
				}
				for _, log := range logs.Worklogs {
					id := issue.Key + ":" + log.ID
					if log.ID == "" || seenLogs[id] {
						return nil, fmt.Errorf("Jira вернула повторную страницу списаний")
					}
					seenLogs[id] = true
					author := log.Author.Key
					if j.cloud() {
						author = log.Author.AccountID
					}
					if author == "" {
						author = log.Author.Name
					}
					member := aliases[author]
					if member == "" {
						continue
					}
					stamp := providerTime(log.Started)
					if stamp.IsZero() || log.TimeSpentSeconds < 0 {
						return nil, fmt.Errorf("Некорректная дата или длительность списания Jira")
					}
					date := stamp.In(loc).Format("2006-01-02")
					if date < from || date > to {
						continue
					}
					out = append(out, domain.Worklog{ID: id, MemberID: member, IssueKey: issue.Key, Title: issue.Fields.Summary, URL: strings.TrimRight(j.URL, "/") + "/browse/" + url.PathEscape(issue.Key), Date: date, Started: stamp, Seconds: log.TimeSpentSeconds, Comment: adfText(log.Comment)})
					if len(out) > 100000 {
						return nil, fmt.Errorf("Слишком много списаний; сократите период")
					}
				}
				offset += len(logs.Worklogs)
				if offset >= logs.Total {
					complete = true
					break
				}
				if len(logs.Worklogs) == 0 {
					return nil, fmt.Errorf("Неполная страница списаний Jira")
				}
			}
			if !complete {
				return nil, fmt.Errorf("Слишком много списаний у одной задачи")
			}
		}
		start += len(res.Issues)
		if !j.cloud() {
			if start >= res.Total {
				return out, nil
			}
			if len(res.Issues) == 0 {
				return nil, fmt.Errorf("Неполная страница задач Jira")
			}
		} else {
			if res.IsLast || res.NextPageToken == "" {
				return out, nil
			}
			if res.NextPageToken == token {
				return nil, fmt.Errorf("Повторный маркер страницы Jira")
			}
			token = res.NextPageToken
		}
	}
	return nil, fmt.Errorf("Слишком много задач со списаниями; сократите период")
}

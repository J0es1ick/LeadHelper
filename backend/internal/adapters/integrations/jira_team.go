package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"leadhelper/internal/domain"
	"net/url"
	"regexp"
	"strings"
)

func (j *Jira) TeamScoped() bool { return j.Scope != "jql" }
func (j *Jira) SiteID() string {
	sum := sha256.Sum256([]byte(strings.TrimRight(j.URL, "/") + fmt.Sprint(j.cloud())))
	return fmt.Sprintf("jira-%x", sum[:12])
}

type jiraUserRecord struct {
	AccountID   string `json:"accountId"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Email       string `json:"emailAddress"`
	Active      bool   `json:"active"`
	AccountType string `json:"accountType"`
}

func (j *Jira) userRecord(r jiraUserRecord) (domain.JiraUser, bool) {
	id, login := r.Key, r.Name
	if id == "" {
		id = r.Name
	}
	if j.cloud() {
		id, login = r.AccountID, r.AccountID
	}
	return domain.JiraUser{ID: id, Login: login, Name: r.DisplayName, Email: r.Email}, r.Active && id != "" && login != "" && (r.AccountType == "" || r.AccountType == "atlassian")
}
func (j *Jira) userRequest(ctx context.Context, path string, query url.Values, out any) error {
	if e := httpsURL(j.URL); e != nil {
		return e
	}
	prefix, auth := "/rest/api/2/", "Bearer "+j.Token
	if j.cloud() {
		prefix, auth = "/rest/api/3/", "Basic "+base64.StdEncoding.EncodeToString([]byte(j.Email+":"+j.Token))
	}
	c := j.Client
	if c == nil {
		c = client()
	}
	return request(ctx, c, "GET", strings.TrimRight(j.URL, "/")+prefix+path+"?"+query.Encode(), nil, map[string]string{"Authorization": auth}, out)
}
func (j *Jira) SearchUsers(ctx context.Context, search string) ([]domain.JiraUser, error) {
	out := []domain.JiraUser{}
	search = strings.TrimSpace(search)
	if len(search) < 2 || len(search) > 200 {
		return out, fmt.Errorf("Введите от 2 до 200 символов для поиска пользователя")
	}
	key := "username"
	if j.cloud() {
		key = "query"
	}
	q := url.Values{key: {search}, "startAt": {"0"}, "maxResults": {"50"}, "includeActive": {"true"}, "includeInactive": {"false"}}
	var rows []jiraUserRecord
	if e := j.userRequest(ctx, "user/search", q, &rows); e != nil {
		return nil, fmt.Errorf("Поиск пользователей Jira: %w. Нужны права просмотра пользователей", e)
	}
	if len(rows) >= 50 {
		return nil, fmt.Errorf("Слишком много совпадений: уточните email или логин")
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if u, ok := j.userRecord(row); ok && !seen[u.ID] {
			out = append(out, u)
			seen[u.ID] = true
		}
	}
	return out, nil
}
func (j *Jira) User(ctx context.Context, id string) (domain.JiraUser, error) {
	var row jiraUserRecord
	key := "key"
	if j.cloud() {
		key = "accountId"
	}
	if e := j.userRequest(ctx, "user", url.Values{key: {id}}, &row); e != nil {
		return domain.JiraUser{}, e
	}
	u, ok := j.userRecord(row)
	if !ok || u.ID != id {
		return domain.JiraUser{}, fmt.Errorf("Пользователь Jira недоступен или неактивен")
	}
	return u, nil
}

var orderBy = regexp.MustCompile(`(?i)^order\s+by\b`)

// Only a top-level ORDER BY outside quoted strings is separated from the filter.
func teamJQL(extra string, logins []string) (string, error) {
	if len(logins) == 0 {
		return "", fmt.Errorf("Нет сопоставленных сотрудников команды; запрос всех задач запрещён")
	}
	where, order := strings.TrimSpace(extra), "ORDER BY updated DESC, key ASC"
	quote := byte(0)
	depth := 0
	escaped := false
	for i := 0; i < len(where); i++ {
		c := where[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '(' {
			depth++
		}
		if c == ')' {
			depth--
			if depth < 0 {
				return "", fmt.Errorf("Незакрытые скобки в JQL")
			}
		}
		if depth == 0 && (i == 0 || where[i-1] == ' ' || where[i-1] == '\n' || where[i-1] == '\t') && orderBy.MatchString(where[i:]) {
			order = where[i:]
			where = strings.TrimSpace(where[:i])
			break
		}
	}
	if quote != 0 || depth != 0 {
		return "", fmt.Errorf("Незакрытые кавычки или скобки в JQL")
	}
	values := []string{}
	seen := map[string]bool{}
	for _, v := range logins {
		if v == "" || strings.ContainsAny(v, "\r\n\x00") {
			return "", fmt.Errorf("Некорректный пользователь Jira")
		}
		if !seen[v] {
			values = append(values, `"`+strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v)+`"`)
			seen[v] = true
		}
	}
	filter := "assignee IN (" + strings.Join(values, ", ") + ")"
	if where != "" {
		filter += " AND (" + where + ")"
	}
	return filter + " " + order, nil
}
func (j *Jira) FetchTeamTasks(ctx context.Context, members []domain.Member) ([]domain.Task, error) {
	ids := []string{}
	owners := map[string]string{}
	for _, m := range members {
		if m.Inactive || m.Demo || m.Source != "bitrix" || m.JiraSite != j.SiteID() || m.JiraUserID == "" || m.JiraLogin == "" {
			return nil, fmt.Errorf("Сначала сопоставьте всех действующих сотрудников с Jira")
		}
		for _, alias := range []string{m.JiraUserID, m.JiraLogin} {
			key := "jira-" + alias
			if old := owners[key]; old != "" && old != m.ID {
				return nil, fmt.Errorf("Один пользователь Jira связан с несколькими сотрудниками")
			}
			owners[key] = m.ID
		}
		ids = append(ids, m.JiraLogin)
	}
	q, e := teamJQL(j.JQL, ids)
	if e != nil {
		return nil, e
	}
	tasks, _, e := j.fetchTasks(ctx, q, owners)
	return tasks, e
}

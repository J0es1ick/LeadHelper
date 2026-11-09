package integrations

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"leadhelper/internal/domain"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type jiraHTTPError struct {
	code    int
	message string
}

func (e *jiraHTTPError) Error() string { return e.message }
func (j *Jira) apiPrefix() string {
	if j.cloud() {
		return "/rest/api/3/"
	}
	return "/rest/api/2/"
}
func (j *Jira) authorization() string {
	if j.cloud() {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(j.Email+":"+j.Token))
	}
	return "Bearer " + j.Token
}
func (j *Jira) jsonRequest(ctx context.Context, method, path string, body, out any) error {
	if e := httpsURL(j.URL); e != nil {
		return e
	}
	var data []byte
	var e error
	if body != nil {
		data, e = json.Marshal(body)
		if e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(j.URL, "/")+j.apiPrefix()+path, bytes.NewReader(data))
	if e != nil {
		return fmt.Errorf("Некорректный запрос Jira")
	}
	req.Header.Set("Authorization", j.authorization())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	c := j.Client
	if c == nil {
		c = client()
	}
	res, e := c.Do(req)
	if e != nil {
		return fmt.Errorf("Не удалось получить ответ Jira")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var detail struct {
			ErrorMessages []string          `json:"errorMessages"`
			Errors        map[string]string `json:"errors"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&detail)
		parts := []string{fmt.Sprintf("Jira: HTTP %d", res.StatusCode)}
		keys := []string{}
		for k := range detail.Errors {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, k+": "+detail.Errors[k])
		}
		parts = append(parts, detail.ErrorMessages...)
		msg := strings.Join(parts, ". ")
		if len(msg) > 3000 {
			msg = msg[:3000]
		}
		if j.Token != "" {
			msg = strings.ReplaceAll(msg, j.Token, "[скрыто]")
		}
		return &jiraHTTPError{res.StatusCode, msg}
	}
	if out == nil {
		return nil
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 20<<20)).Decode(out); e != nil {
		return fmt.Errorf("Jira вернула некорректный ответ")
	}
	return nil
}
func (j *Jira) CreationProjects(ctx context.Context) ([]domain.JiraOption, error) {
	out := []domain.JiraOption{}
	if !j.cloud() {
		e := j.jsonRequest(ctx, "GET", "project", nil, &out)
		return out, e
	}
	for start := 0; start < 10000; {
		var res struct {
			Values []domain.JiraOption
			Total  int
			IsLast bool
		}
		if e := j.jsonRequest(ctx, "GET", fmt.Sprintf("project/search?maxResults=100&startAt=%d&action=create", start), nil, &res); e != nil {
			return nil, e
		}
		out = append(out, res.Values...)
		start += len(res.Values)
		if res.IsLast || start >= res.Total {
			return out, nil
		}
		if len(res.Values) == 0 {
			break
		}
	}
	return nil, fmt.Errorf("Не удалось полностью загрузить проекты Jira")
}
func (j *Jira) metadataRows(ctx context.Context, path string) ([]map[string]any, error) {
	out := []map[string]any{}
	seen := map[string]bool{}
	for start := 0; start < 5000; {
		var res struct {
			Values     []map[string]any
			Fields     []map[string]any
			IssueTypes []map[string]any
			Total      int
			IsLast     bool
		}
		if e := j.jsonRequest(ctx, "GET", path+"?maxResults=100&startAt="+strconv.Itoa(start), nil, &res); e != nil {
			return nil, e
		}
		rows := res.Values
		if rows == nil {
			rows = res.Fields
		}
		if rows == nil {
			rows = res.IssueTypes
		}
		for _, r := range rows {
			id := str(r["fieldId"])
			if id == "" {
				id = str(r["id"])
			}
			if id == "" {
				id = str(r["key"])
			}
			if id == "" || seen[id] {
				return nil, fmt.Errorf("Jira вернула повторную страницу полей")
			}
			seen[id] = true
		}
		out = append(out, rows...)
		start += len(rows)
		if res.IsLast || start >= res.Total {
			return out, nil
		}
		if len(rows) == 0 {
			break
		}
	}
	return nil, fmt.Errorf("Слишком много полей Jira; откройте форму в Jira")
}
func (j *Jira) legacyMeta(ctx context.Context, project, typ string) ([]map[string]any, error) {
	q := url.Values{"projectIds": {project}, "expand": {"projects.issuetypes.fields"}}
	if typ != "" {
		q.Set("issuetypeIds", typ)
	}
	var res struct {
		Projects []struct {
			IssueTypes []map[string]any `json:"issuetypes"`
		}
	}
	if e := j.jsonRequest(ctx, "GET", "issue/createmeta?"+q.Encode(), nil, &res); e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, p := range res.Projects {
		out = append(out, p.IssueTypes...)
	}
	return out, nil
}
func oldMetadata(e error) bool {
	var h *jiraHTTPError
	return errors.As(e, &h) && (h.code == 404 || h.code == 405)
}
func (j *Jira) CreationTypes(ctx context.Context, project string) ([]domain.JiraOption, error) {
	rows, e := j.metadataRows(ctx, "issue/createmeta/"+url.PathEscape(project)+"/issuetypes")
	if oldMetadata(e) && !j.cloud() {
		rows, e = j.legacyMeta(ctx, project, "")
	}
	if e != nil {
		return nil, e
	}
	out := []domain.JiraOption{}
	for _, r := range rows {
		sub, _ := r["subtask"].(bool)
		out = append(out, domain.JiraOption{ID: str(r["id"]), Name: str(r["name"]), Subtask: sub})
	}
	return out, nil
}
func (j *Jira) CreationFields(ctx context.Context, project, typ string) (domain.JiraCreateMeta, error) {
	out := domain.JiraCreateMeta{Fields: []domain.JiraCreateField{}, Cloud: j.cloud(), URL: strings.TrimRight(j.URL, "/") + "/secure/CreateIssue!default.jspa?pid=" + url.QueryEscape(project) + "&issuetype=" + url.QueryEscape(typ)}
	rows, e := j.metadataRows(ctx, "issue/createmeta/"+url.PathEscape(project)+"/issuetypes/"+url.PathEscape(typ))
	if oldMetadata(e) && !j.cloud() {
		var types []map[string]any
		types, e = j.legacyMeta(ctx, project, typ)
		rows = nil
		for _, t := range types {
			for id, v := range obj(t["fields"]) {
				m := obj(v)
				m["fieldId"] = id
				rows = append(rows, m)
			}
		}
	}
	if e != nil {
		return out, e
	}
	for _, r := range rows {
		id := str(r["fieldId"])
		if id == "" {
			id = str(r["key"])
		}
		if id == "" {
			continue
		}
		required, _ := r["required"].(bool)
		f := domain.JiraCreateField{ID: id, Name: str(r["name"]), Required: required, Schema: obj(r["schema"]), Default: r["defaultValue"]}
		if a, ok := r["allowedValues"].([]any); ok {
			for _, v := range a {
				f.Allowed = append(f.Allowed, obj(v))
			}
		}
		out.Fields = append(out.Fields, f)
	}
	sort.Slice(out.Fields, func(a, b int) bool { return out.Fields[a].ID < out.Fields[b].ID })
	return out, nil
}
func (j *Jira) AssignableUsers(ctx context.Context, project, search string) ([]domain.JiraUser, error) {
	key := "username"
	if j.cloud() {
		key = "query"
	}
	q := url.Values{"project": {project}, key: {search}, "maxResults": {"50"}}
	var rows []jiraUserRecord
	if e := j.userRequest(ctx, "user/assignable/search", q, &rows); e != nil {
		return nil, e
	}
	out := []domain.JiraUser{}
	for _, r := range rows {
		if u, ok := j.userRecord(r); ok {
			out = append(out, u)
		}
	}
	return out, nil
}
func jiraDocument(text string) any {
	content := []any{}
	for _, line := range strings.Split(text, "\n") {
		p := map[string]any{"type": "paragraph"}
		if line != "" {
			p["content"] = []any{map[string]any{"type": "text", "text": line}}
		}
		content = append(content, p)
	}
	return map[string]any{"type": "doc", "version": 1, "content": content}
}
func (j *Jira) CreateIssue(ctx context.Context, in domain.JiraCreateInput) (domain.JiraCreateResult, error) {
	out := domain.JiraCreateResult{Warnings: []string{}}
	meta, e := j.CreationFields(ctx, in.Project, in.IssueType)
	if e != nil {
		return out, &domain.JiraWriteError{Message: e.Error(), Definitive: true}
	}
	allowed := map[string]domain.JiraCreateField{}
	for _, f := range meta.Fields {
		allowed[f.ID] = f
	}
	fields := map[string]any{"project": map[string]string{"id": in.Project}, "issuetype": map[string]string{"id": in.IssueType}}
	for k, v := range in.Fields {
		f, ok := allowed[k]
		if (!ok && k != "parent") || k == "project" || k == "issuetype" || k == "attachment" {
			return out, &domain.JiraWriteError{Message: "Поле недоступно при создании: " + k, Definitive: true}
		}
		if j.cloud() && (k == "description" || k == "environment" || strings.HasSuffix(str(f.Schema["custom"]), ":textarea")) {
			if text, ok := v.(string); ok {
				v = jiraDocument(text)
			}
		}
		fields[k] = v
	}
	var created struct {
		Key string
		ID  string
	}
	e = j.jsonRequest(ctx, "POST", "issue", map[string]any{"fields": fields}, &created)
	if e != nil {
		var h *jiraHTTPError
		def := errors.As(e, &h) && h.code >= 400 && h.code < 500 && h.code != 408
		return out, &domain.JiraWriteError{Message: e.Error(), Definitive: def}
	}
	if created.Key == "" {
		return out, &domain.JiraWriteError{Message: "Jira не вернула ключ созданной задачи. Проверьте Jira перед повторным созданием"}
	}
	now := time.Now().UTC()
	out.Task = domain.Task{ID: "jira-" + created.Key, ExternalID: created.Key, Source: "jira", Title: str(fields["summary"]), Description: adfText(fields["description"]), Status: "todo", Priority: "medium", Project: in.Project, URL: strings.TrimRight(j.URL, "/") + "/browse/" + url.PathEscape(created.Key), Tags: []string{}, CreatedAt: now, UpdatedAt: now, DueDate: str(fields["duedate"])}
	// Read the actual workflow status and estimates; the issue is already created even if this read fails.
	var detail struct {
		Fields struct {
			Summary              string
			Description          any
			Project              struct{ Name string }
			Assignee             *jiraUserRecord
			Priority             struct{ Name string }
			Labels               []string
			Status               struct{ StatusCategory struct{ Key string } }
			TimeOriginalEstimate float64
		}
	}
	if e = j.jsonRequest(ctx, "GET", "issue/"+url.PathEscape(created.Key)+"?fields=summary,description,project,status,timeoriginalestimate,assignee,priority,labels", nil, &detail); e == nil {
		out.Task.Title = detail.Fields.Summary
		out.Task.Description = adfText(detail.Fields.Description)
		out.Task.Project = detail.Fields.Project.Name
		out.Task.Estimate = detail.Fields.TimeOriginalEstimate / 3600
		if detail.Fields.Labels != nil {
			out.Task.Tags = detail.Fields.Labels
		}
		if a := detail.Fields.Assignee; a != nil {
			id := a.Key
			if id == "" {
				id = a.Name
			}
			if j.cloud() {
				id = a.AccountID
			}
			if id != "" {
				out.Task.AssigneeID = "jira-" + id
			}
		}
		switch strings.ToLower(detail.Fields.Priority.Name) {
		case "highest", "critical", "blocker", "критический", "блокирующий", "самый высокий":
			out.Task.Priority = "critical"
		case "high", "высокий":
			out.Task.Priority = "high"
		case "low", "lowest", "низкий", "самый низкий":
			out.Task.Priority = "low"
		}
		switch detail.Fields.Status.StatusCategory.Key {
		case "done":
			out.Task.Status = "done"
		case "indeterminate":
			out.Task.Status = "progress"
		}
	} else {
		out.Warnings = append(out.Warnings, "Задача создана; подробности обновятся при синхронизации Jira")
	}
	return out, nil
}
func (j *Jira) UploadIssueFiles(ctx context.Context, key string, files []domain.JiraUpload) []string {
	warnings := []string{}
	for _, file := range files {
		data, e := base64.StdEncoding.DecodeString(file.Data)
		if e != nil {
			warnings = append(warnings, "Не удалось прочитать файл: "+file.Name)
			continue
		}
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, e := writer.CreateFormFile("file", file.Name)
		if e != nil {
			warnings = append(warnings, "Некорректное имя файла")
			continue
		}
		_, _ = part.Write(data)
		_ = writer.Close()
		req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(j.URL, "/")+j.apiPrefix()+"issue/"+url.PathEscape(key)+"/attachments", &body)
		if e != nil {
			warnings = append(warnings, "Файл не загружен: "+file.Name)
			continue
		}
		req.Header.Set("Authorization", j.authorization())
		req.Header.Set("X-Atlassian-Token", "no-check")
		req.Header.Set("Content-Type", writer.FormDataContentType())
		c := j.Client
		if c == nil {
			c = client()
		}
		res, e := c.Do(req)
		if e != nil {
			warnings = append(warnings, "Проверьте наличие файла в Jira: "+file.Name)
			continue
		}
		res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			warnings = append(warnings, "Jira не приняла вложение: "+file.Name)
		}
	}
	return warnings
}

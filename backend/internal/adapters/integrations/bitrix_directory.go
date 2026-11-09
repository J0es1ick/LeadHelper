package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"sort"
	"strconv"
	"strings"
)

func bitrixBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	s := strings.ToLower(str(v))
	return s == "y" || s == "true" || s == "1"
}
func bitrixIDs(v any) []string {
	out := []string{}
	if list, ok := v.([]any); ok {
		for _, x := range list {
			if id := str(x); id != "" && id != "0" {
				out = append(out, id)
			}
		}
	} else if id := str(v); id != "" && id != "0" {
		out = append(out, id)
	}
	return out
}
func userName(r map[string]any) string {
	return strings.TrimSpace(str(r["NAME"]) + " " + str(r["LAST_NAME"]))
}
func (b *Bitrix) people(ctx context.Context, q domain.RemoteQuery) (domain.RemotePage, error) {
	out := domain.RemotePage{Items: []domain.RemoteItem{}}
	start, e := remoteStart(q.Cursor)
	if e != nil {
		return out, e
	}
	filter := map[string]any{"USER_TYPE": "employee"}
	switch q.Status {
	case "", "all":
	case "active":
		filter["ACTIVE"] = true
	case "dismissed":
		filter["ACTIVE"] = false
	case "online":
		filter["ACTIVE"] = true
		filter["IS_ONLINE"] = "Y"
	default:
		return out, errors.New("Неизвестный статус сотрудника")
	}
	if q.Department != "" {
		n, e := strconv.Atoi(q.Department)
		if e != nil || n <= 0 {
			return out, errors.New("Некорректный отдел")
		}
		filter["UF_DEPARTMENT"] = n
	}
	if strings.TrimSpace(q.Search) != "" {
		filter["NAME_SEARCH"] = strings.TrimSpace(q.Search)
	}
	var rows []map[string]any
	meta, e := b.callPage(ctx, "user.get", map[string]any{"FILTER": filter, "SORT": "ID", "ORDER": "ASC", "start": start}, &rows)
	if e != nil {
		return out, e
	}
	out.Next = meta.Next
	out.Total = meta.Total
	if len(filter) == 1 && start == 0 {
		out.TotalAll = meta.Total
	} else {
		var countRows []map[string]any
		count, e := b.callPage(ctx, "user.get", map[string]any{"FILTER": map[string]any{"USER_TYPE": "employee"}, "start": 0}, &countRows)
		if e == nil {
			out.TotalAll = count.Total
		} else {
			out.Warning = "Общее число сотрудников временно недоступно"
		}
	}
	for _, r := range rows {
		id := str(r["ID"])
		status := "dismissed"
		if bitrixBool(r["ACTIVE"]) {
			status = "active"
		}
		title := userName(r)
		if title == "" {
			title = "Сотрудник " + id
		}
		out.Items = append(out.Items, domain.RemoteItem{ID: id, Title: title, AvatarURL: b.avatarURL(r["PERSONAL_PHOTO"]), Text: str(r["WORK_POSITION"]), Kind: "people", Status: status, Online: bitrixBool(r["IS_ONLINE"]), Departments: bitrixIDs(r["UF_DEPARTMENT"]), URL: b.base() + "/company/personal/user/" + id + "/"})
	}
	return out, nil
}
func (b *Bitrix) userNames(ctx context.Context, ids []string) (map[string]string, error) {
	people, e := b.userProfiles(ctx, ids)
	names := map[string]string{}
	for id, person := range people {
		names[id] = person.Title
	}
	return names, e
}
func (b *Bitrix) userProfiles(ctx context.Context, ids []string) (map[string]domain.RemoteItem, error) {
	people := map[string]domain.RemoteItem{}
	unique := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && id != "0" && !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	for offset := 0; offset < len(unique); offset += 50 {
		var rows []map[string]any
		_, e := b.call(ctx, "user.get", map[string]any{"FILTER": map[string]any{"ID": unique[offset:min(offset+50, len(unique))]}}, &rows)
		if e != nil {
			return people, e
		}
		for _, r := range rows {
			people[str(r["ID"])] = domain.RemoteItem{Title: userName(r), AvatarURL: b.avatarURL(r["PERSONAL_PHOTO"])}
		}
	}
	return people, nil
}
func (b *Bitrix) departments(ctx context.Context) (domain.RemotePage, error) {
	out := domain.RemotePage{Items: []domain.RemoteItem{}}
	cursor := ""
	seen := map[string]bool{}
	heads := []string{}
	for page := 0; page < 100; page++ {
		start, e := remoteStart(cursor)
		if e != nil {
			return out, e
		}
		var rows []map[string]any
		next, e := b.call(ctx, "department.get", map[string]any{"sort": "SORT", "order": "ASC", "start": start}, &rows)
		if e != nil {
			return out, e
		}
		for _, r := range rows {
			id := str(r["ID"])
			if seen[id] {
				continue
			}
			seen[id] = true
			head := str(r["UF_HEAD"])
			if head == "0" {
				head = ""
			}
			heads = append(heads, head)
			out.Items = append(out.Items, domain.RemoteItem{ID: id, Title: str(r["NAME"]), Parent: str(r["PARENT"]), HeadID: head, Sort: integer(r["SORT"]), Kind: "departments"})
		}
		if next == "" {
			break
		}
		if next == cursor || len(rows) == 0 {
			return out, errors.New("Битрикс24 вернул повторную страницу структуры")
		}
		cursor = next
		if page == 99 {
			return out, errors.New("Структура содержит более 5000 отделов. Не удалось загрузить её полностью")
		}
	}
	people, e := b.userProfiles(ctx, heads)
	if e != nil {
		out.Warning = "Структура загружена, но имена руководителей недоступны. Проверьте право «Пользователи» у вебхука"
	}
	for i := range out.Items {
		d := &out.Items[i]
		d.HeadName = people[d.HeadID].Title
		d.HeadAvatarURL = people[d.HeadID].AvatarURL
		if d.HeadID != "" {
			d.HeadURL = b.base() + "/company/personal/user/" + d.HeadID + "/"
		}
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		if out.Items[i].Sort == out.Items[j].Sort {
			return out.Items[i].Title < out.Items[j].Title
		}
		return out.Items[i].Sort < out.Items[j].Sort
	})
	n := len(out.Items)
	out.Total = &n
	return out, nil
}
func (b *Bitrix) news(ctx context.Context, q domain.RemoteQuery) (domain.RemotePage, error) {
	out := domain.RemotePage{Items: []domain.RemoteItem{}}
	last, e := remoteStart(q.Cursor)
	if e != nil {
		return out, e
	}
	var raw json.RawMessage
	meta, e := b.callPage(ctx, "log.blogpost.get", map[string]any{"LAST_ID": last, "LIMIT": 30}, &raw)
	if e != nil {
		return out, e
	}
	out.Next = meta.Next
	var rows []map[string]any
	if e = json.Unmarshal(raw, &rows); e != nil {
		var values map[string]json.RawMessage
		if e = json.Unmarshal(raw, &values); e != nil {
			return out, errors.New("Битрикс24: неверный формат ленты")
		}
		// The feed cursor is a log-entry ID, not the blog-post ID.
		if rawNext, ok := values["next"]; ok {
			var value any
			if json.Unmarshal(rawNext, &value) != nil {
				return out, errors.New("Битрикс24: неверный курсор ленты")
			}
			next, e := remoteStart(str(value))
			if e != nil || next <= 0 || (last > 0 && next >= last) {
				return out, errors.New("Битрикс24 вернул повторную страницу ленты")
			}
			out.Next = strconv.Itoa(next)
		}
		for key, value := range values {
			if _, e = strconv.Atoi(key); e == nil {
				var row map[string]any
				if json.Unmarshal(value, &row) == nil {
					rows = append(rows, row)
				}
			}
		}
	}
	if out.Next != "" {
		next, e := remoteStart(out.Next)
		if e != nil || next <= 0 || (last > 0 && next >= last) {
			return out, errors.New("Битрикс24 вернул повторную страницу ленты")
		}
	}
	authors := []string{}
	seen := map[string]bool{}
	for _, r := range rows {
		id := integer(r["ID"])
		if id <= 0 || seen[str(r["ID"])] {
			continue
		}
		seen[str(r["ID"])] = true
		author := str(r["AUTHOR_ID"])
		authors = append(authors, author)
		out.Items = append(out.Items, domain.RemoteItem{ID: strconv.Itoa(id), Title: str(r["TITLE"]), Text: str(r["DETAIL_TEXT"]), Author: author, Date: providerTime(str(r["DATE_PUBLISH"])), Kind: "news", URL: b.base() + "/company/personal/user/" + author + "/blog/" + strconv.Itoa(id) + "/"})
	}
	people, _ := b.userProfiles(ctx, authors)
	for i := range out.Items {
		person := people[out.Items[i].Author]
		out.Items[i].AvatarURL = person.AvatarURL
		if name := person.Title; name != "" {
			out.Items[i].Author = name
		} else if out.Items[i].Author != "" {
			out.Items[i].Author = fmt.Sprintf("Сотрудник %s", out.Items[i].Author)
		}
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		if out.Items[i].Date.Equal(out.Items[j].Date) {
			a, _ := strconv.Atoi(out.Items[i].ID)
			z, _ := strconv.Atoi(out.Items[j].ID)
			return a > z
		}
		return out.Items[i].Date.After(out.Items[j].Date)
	})
	return out, nil
}

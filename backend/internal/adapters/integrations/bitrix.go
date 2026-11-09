package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

type Bitrix struct {
	Webhook string
	Client  *http.Client
}

func (b *Bitrix) Configured() bool { return b != nil && b.Webhook != "" }
func (b *Bitrix) AccountID() string {
	u, _ := url.Parse(b.Webhook)
	if u == nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	h := sha256.Sum256([]byte(u.Host + "/" + parts[1]))
	return "bitrix-" + hex.EncodeToString(h[:12])
}

var webhookPath = regexp.MustCompile(`^/rest/[1-9][0-9]*/[A-Za-z0-9_-]+/?$`)
var remoteID = regexp.MustCompile(`^(?:chat|sg)?[1-9][0-9]*$`)

func (b *Bitrix) base() string {
	u, _ := url.Parse(b.Webhook)
	if u == nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
func (b *Bitrix) userID() string {
	u, _ := url.Parse(b.Webhook)
	if u == nil {
		return ""
	}
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(p) < 2 {
		return ""
	}
	return p[1]
}

type bitrixPageMeta struct {
	Next  string
	Total *int
}

func (b *Bitrix) call(ctx context.Context, method string, body any, out any) (string, error) {
	meta, e := b.callPage(ctx, method, body, out)
	return meta.Next, e
}
func (b *Bitrix) callPage(ctx context.Context, method string, body any, out any) (bitrixPageMeta, error) {
	if !b.Configured() {
		return bitrixPageMeta{}, errors.New("Битрикс24 не подключён. Добавьте входящий вебхук в Интеграциях")
	}
	u, e := url.Parse(b.Webhook)
	if e != nil || httpsURL(b.Webhook) != nil || !webhookPath.MatchString(u.Path) || u.RawQuery != "" {
		return bitrixPageMeta{}, errors.New("Битрикс24: нужен HTTPS URL входящего вебхука /rest/USER/KEY/")
	}
	c := b.Client
	if c == nil {
		c = client()
	}
	var response struct {
		Result json.RawMessage `json:"result"`
		Next   *int            `json:"next"`
		Total  *int            `json:"total"`
		Error  string          `json:"error"`
	}
	if e = request(ctx, c, "POST", strings.TrimRight(b.Webhook, "/")+"/"+method+".json", body, nil, &response); e != nil {
		return bitrixPageMeta{}, e
	}
	if response.Error != "" {
		return bitrixPageMeta{}, fmt.Errorf("Битрикс24: метод %s недоступен. Проверьте срок вебхука, права и тариф портала", method)
	}
	if e = json.Unmarshal(response.Result, out); e != nil {
		return bitrixPageMeta{}, fmt.Errorf("Битрикс24: неожиданный ответ метода %s", method)
	}
	if response.Next != nil {
		return bitrixPageMeta{Next: strconv.Itoa(*response.Next), Total: response.Total}, nil
	}
	return bitrixPageMeta{Total: response.Total}, nil
}
func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	}
	return ""
}
func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }
func integer(v any) int        { n, _ := strconv.Atoi(str(v)); return n }
func remoteStart(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n, e := strconv.Atoi(s)
	if e != nil || n < 0 || n > 10000000 {
		return 0, errors.New("Некорректная страница")
	}
	return n, nil
}
func (b *Bitrix) identity(ctx context.Context) (map[string]any, error) {
	var p map[string]any
	_, e := b.call(ctx, "profile", map[string]any{}, &p)
	return p, e
}
func (b *Bitrix) Browse(ctx context.Context, q domain.RemoteQuery) (domain.RemotePage, error) {
	out := domain.RemotePage{Items: []domain.RemoteItem{}, AccountID: b.AccountID()}
	args := map[string]any{}
	var rows []map[string]any
	method := ""
	switch q.Kind {
	case "chats":
		args["LIMIT"] = 50
		args["PARSE_TEXT"] = "N"
		args["GET_ORIGINAL_TEXT"] = "Y"
		if q.Cursor != "" {
			if _, e := time.Parse(time.RFC3339, q.Cursor); e != nil {
				return out, errors.New("Некорректная страница чатов")
			}
			args["LAST_MESSAGE_DATE"] = q.Cursor
		}
		var res struct {
			Items        []map[string]any
			HasMore      bool
			HasMorePages bool
		}
		if _, e := b.call(ctx, "im.recent.list", args, &res); e != nil {
			return out, e
		}
		for _, r := range res.Items {
			msg := obj(r["message"])
			id := str(r["id"])
			if str(r["type"]) == "chat" && !strings.HasPrefix(id, "chat") {
				id = "chat" + id
			}
			title := str(r["title"])
			if title == "" {
				title = str(obj(r["user"])["name"])
			}
			if title == "" {
				title = "Диалог " + id
			}
			avatar := b.avatarURL(r["avatar"], obj(r["user"])["avatar"], obj(r["chat"])["avatar"])
			out.Items = append(out.Items, domain.RemoteItem{ID: id, Title: title, AvatarURL: avatar, Text: recentMessageText(msg), Author: str(msg["author_id"]), Date: providerTime(str(msg["date"])), Unread: integer(r["counter"]), MessageID: str(msg["id"]), ReadMessageID: str(r["last_id"]), Own: str(msg["author_id"]) == b.userID(), URL: b.base() + "/online/?IM_DIALOG=" + url.QueryEscape(id), Kind: "chat"})
		}
		if (res.HasMore || res.HasMorePages) && len(out.Items) > 0 {
			out.Next = out.Items[len(out.Items)-1].Date.Format(time.RFC3339)
		}
		return out, nil
	case "messages", "message":
		if !remoteID.MatchString(q.ID) {
			return out, errors.New("Некорректный диалог")
		}
		args["DIALOG_ID"] = q.ID
		args["LIMIT"] = 50
		if q.Cursor != "" && q.Kind != "message" {
			n, e := remoteStart(q.Cursor)
			if e != nil {
				return out, e
			}
			args["LAST_ID"] = n
		}
		if q.Kind == "message" {
			n, e := strconv.ParseInt(q.Cursor, 10, 64)
			if e != nil || n < 1 || n >= 1<<53 {
				return out, errors.New("Некорректное сообщение")
			}
			args["LAST_ID"] = n + 1
			args["LIMIT"] = 1
		}
		var res struct {
			Messages []map[string]any
			Users    []map[string]any
			Files    []map[string]any
		}
		if _, e := b.call(ctx, "im.dialog.messages.get", args, &res); e != nil {
			return out, e
		}
		names := map[string]string{}
		avatars := map[string]string{}
		for _, u := range res.Users {
			names[str(u["id"])] = str(u["name"])
			avatars[str(u["id"])] = b.avatarURL(u["avatar"], u["avatar_hr"])
		}
		min := int(^uint(0) >> 1)
		for _, m := range res.Messages {
			if q.Kind == "message" && str(m["id"]) != q.Cursor {
				continue
			}
			n := integer(m["id"])
			if n > 0 && n < min {
				min = n
			}
			author := names[str(m["author_id"])]
			if author == "" {
				author = "Участник " + str(m["author_id"])
			}
			if str(m["author_id"]) == "0" {
				author = "Системное сообщение"
				if strings.Contains(str(m["text"]), "BitrixGPT") {
					author = "BitrixGPT"
				}
			}
			files := b.messageFiles(m, res.Files)
			out.Items = append(out.Items, domain.RemoteItem{ID: str(m["id"]), Title: author, AvatarURL: avatars[str(m["author_id"])], System: author == "Системное сообщение", Text: messageFileText(str(m["text"]), files), Date: providerTime(str(m["date"])), Kind: "message", Own: str(m["author_id"]) == b.userID(), Attachments: files, Cards: b.messageCards(m), References: b.messageReferences(str(m["text"]))})
		}
		sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].Date.After(out.Items[j].Date) })
		// Bitrix may exceed LIMIT to include unread messages. Keep response bounded.
		if len(out.Items) > 50 {
			out.Items = out.Items[:50]
			min = integer(out.Items[49].ID)
		}
		if len(out.Items) >= 50 {
			out.Next = strconv.Itoa(min)
		}
		return out, nil
	case "news":
		return b.news(ctx, q)
	case "people":
		return b.people(ctx, q)
	case "departments":
		return b.departments(ctx)
	case "files":
		if q.ID == "" {
			method = "disk.storage.getlist"
		} else {
			if !remoteID.MatchString(q.ID) {
				return out, errors.New("Некорректная папка")
			}
			method = "disk.folder.getchildren"
			args["id"] = q.ID
		}
	default:
		return out, errors.New("Неизвестный раздел Битрикс24")
	}
	start, e := remoteStart(q.Cursor)
	if e != nil {
		return out, e
	}
	args["start"] = start
	out.Next, e = b.call(ctx, method, args, &rows)
	if e != nil {
		return out, e
	}
	for _, r := range rows {
		item := domain.RemoteItem{ID: str(r["ID"]), Kind: q.Kind, Title: str(r["NAME"])}
		switch q.Kind {
		case "news":
			item.Title = str(r["TITLE"])
			item.Text = str(r["DETAIL_TEXT"])
			item.Date = providerTime(str(r["DATE_PUBLISH"]))
			item.URL = b.base() + "/company/personal/user/" + str(r["AUTHOR_ID"]) + "/blog/" + item.ID + "/"
		case "people":
			item.Title = strings.TrimSpace(item.Title + " " + str(r["LAST_NAME"]))
			item.Text = str(r["WORK_POSITION"])
			item.URL = b.base() + "/company/personal/user/" + item.ID + "/"
		case "departments":
			item.Parent = str(r["PARENT"])
			item.Text = "Руководитель: " + str(r["UF_HEAD"])
		case "files":
			item.Kind = "file"
			if q.ID == "" {
				item.ID = str(r["ROOT_OBJECT_ID"])
				item.Kind = "folder"
			} else if str(r["TYPE"]) == "folder" {
				item.Kind = "folder"
			}
			item.URL = safeRemoteURL(b.base(), str(r["DETAIL_URL"]))
			item.Text = str(r["SIZE"])
			if item.Kind == "file" && bitrixFileID.MatchString(item.ID) {
				item.Attachments = []domain.RemoteAttachment{b.fileInfo(item.ID, item.Title, int64(integer(r["SIZE"])))}
			}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
func safeRemoteURL(base, raw string) string {
	if raw == "" {
		return ""
	}
	u, e := url.Parse(raw)
	origin, _ := url.Parse(base)
	if e != nil || origin == nil {
		return ""
	}
	u = origin.ResolveReference(u)
	if u.Scheme != "https" || u.Host != origin.Host || u.User != nil || strings.Contains(u.Path, "/rest/") {
		return ""
	}
	return u.String()
}
func (b *Bitrix) SendChat(ctx context.Context, id, body string) error {
	if !remoteID.MatchString(id) || strings.TrimSpace(body) == "" || len(body) > 10000 {
		return errors.New("Укажите диалог и текст до 10 000 символов")
	}
	var result any
	_, e := b.call(ctx, "im.message.add", map[string]any{"DIALOG_ID": id, "MESSAGE": body}, &result)
	return e
}

func bitrixDate(raw, zone string) (time.Time, error) {
	if t, e := time.Parse(time.RFC3339, raw); e == nil {
		return t, nil
	}
	if zone == "" {
		zone = "Europe/Moscow"
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return time.Time{}, errors.New("Битрикс24: неизвестный часовой пояс события")
	}
	for _, layout := range []string{"02.01.2006 15:04:05", "02.01.2006", "2006-01-02 15:04:05", "2006-01-02", "01/02/2006 03:04:05 pm", "01/02/2006 3:04:05 pm"} {
		if t, e := time.ParseInLocation(layout, strings.ToLower(raw), loc); e == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("Битрикс24: не удалось разобрать время события")
}
func (b *Bitrix) Calendar(ctx context.Context, from, to string) (domain.CalendarPage, error) {
	out := domain.CalendarPage{Events: []domain.CalendarEvent{}, Timezone: "Europe/Moscow", UpdatedAt: time.Now().UTC()}
	profile, e := b.identity(ctx)
	if e != nil {
		return out, e
	}
	user := str(profile["ID"])
	if user == "" {
		return out, errors.New("Битрикс24: не определён пользователь")
	}
	if z := str(profile["TIME_ZONE"]); z != "" {
		out.Timezone = z
	}
	var availability map[string][]map[string]any
	if _, e = b.call(ctx, "calendar.accessibility.get", map[string]any{"from": from, "to": to, "users": []string{user}}, &availability); e != nil {
		return out, e
	}
	var details []map[string]any
	if _, e = b.call(ctx, "calendar.event.get", map[string]any{"type": "user", "ownerId": user, "from": from, "to": to}, &details); e != nil {
		return out, e
	}
	detail := map[string]map[string]any{}
	for _, d := range details {
		detail[str(d["ID"])] = d
	}
	seen := map[string]bool{}
	for _, v := range availability[user] {
		d := detail[str(v["ID"])]
		if str(d["DELETED"]) == "Y" || str(d["MEETING_STATUS"]) == "N" || str(d["ACTIVE"]) == "N" {
			continue
		}
		zone := str(v["TZ_FROM"])
		if zone == "" {
			zone = out.Timezone
		}
		start, e := bitrixDate(str(v["DATE_FROM"]), zone)
		if e != nil {
			return out, e
		}
		endzone := str(v["TZ_TO"])
		if endzone == "" {
			endzone = zone
		}
		end, e := bitrixDate(str(v["DATE_TO"]), endzone)
		if e != nil {
			return out, e
		}
		allDay := str(v["DT_SKIP_TIME"]) == "Y"
		if allDay {
			start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
			end = time.Date(end.Year(), end.Month(), end.Day()+1, 0, 0, 0, 0, end.Location())
		}
		if !end.After(start) {
			return out, errors.New("Битрикс24: окончание события раньше начала")
		}
		id := str(v["ID"]) + "-" + start.Format(time.RFC3339)
		if seen[id] {
			continue
		}
		seen[id] = true
		title := str(v["NAME"])
		if title == "" {
			title = "Занято · закрытое событие"
		}
		out.Events = append(out.Events, domain.CalendarEvent{ID: id, Title: title, Start: start, End: end, AllDay: allDay, Busy: str(v["ACCESSIBILITY"]) != "free", Tentative: str(v["ACCESSIBILITY"]) == "quest" || str(d["MEETING_STATUS"]) == "Q", Location: str(d["LOCATION"]), Description: str(d["DESCRIPTION"]), URL: b.base() + "/company/personal/user/" + user + "/calendar/?EVENT_ID=" + url.QueryEscape(str(v["ID"]))})
	}
	sort.Slice(out.Events, func(i, j int) bool { return out.Events[i].Start.Before(out.Events[j].Start) })
	return out, nil
}

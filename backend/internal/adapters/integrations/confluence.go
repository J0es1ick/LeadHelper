package integrations

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"leadhelper/internal/mailbody"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Confluence struct {
	URL, Token string
	Client     *http.Client
}

func (c *Confluence) Configured() bool { return c != nil && c.URL != "" && c.Token != "" }
func (c *Confluence) Browse(ctx context.Context, q domain.RemoteQuery) (domain.RemotePage, error) {
	out := domain.RemotePage{Items: []domain.RemoteItem{}}
	if !c.Configured() {
		return out, errors.New("Confluence не подключён. Добавьте персональный токен в Интеграциях")
	}
	if e := httpsURL(c.URL); e != nil {
		return out, e
	}
	start, e := remoteStart(q.Cursor)
	if e != nil {
		return out, e
	}
	values := url.Values{"start": {strconv.Itoa(start)}, "limit": {"25"}}
	path := "/rest/api/space"
	switch q.Kind {
	case "spaces":
	case "pages":
		path = "/rest/api/content/search"
		cql := "type=page"
		if q.ID != "" {
			cql += " AND space=" + strconv.Quote(q.ID)
		}
		if strings.TrimSpace(q.Search) != "" {
			cql += " AND text~" + strconv.Quote(q.Search)
		}
		values.Set("cql", cql+" ORDER BY lastmodified DESC")
		values.Set("expand", "space,version")
	case "page":
		if !remoteID.MatchString(q.ID) {
			return out, errors.New("Некорректная страница")
		}
		path = "/rest/api/content/" + url.PathEscape(q.ID)
		values.Set("expand", "body.view,space,version")
	default:
		return out, errors.New("Неизвестный раздел Confluence")
	}
	transport := c.Client
	if transport == nil {
		transport = client()
	}
	type page struct {
		ID, Key, Name, Title string
		Links                struct{ WebUI, Next string } `json:"_links"`
		Space                struct{ Name, Key string }
		Version              struct {
			When string
			By   struct{ DisplayName string }
		}
		Body struct{ View struct{ Value string } }
	}
	convert := func(v page) domain.RemoteItem {
		id := v.ID
		title := v.Title
		kind := "page"
		if q.Kind == "spaces" {
			id = v.Key
			title = v.Name
			kind = "space"
		}
		return domain.RemoteItem{ID: id, Title: title, Kind: kind, Text: v.Space.Name, Author: v.Version.By.DisplayName, Date: providerTime(v.Version.When), URL: safeRemoteURL(strings.TrimRight(c.URL, "/")+"/", v.Links.WebUI), HTML: mailbody.Clean(v.Body.View.Value)}
	}
	endpoint := strings.TrimRight(c.URL, "/") + path + "?" + values.Encode()
	headers := map[string]string{"Authorization": "Bearer " + c.Token}
	if q.Kind == "page" {
		var v page
		if e = request(ctx, transport, "GET", endpoint, nil, headers, &v); e != nil {
			return out, e
		}
		out.Items = append(out.Items, convert(v))
		return out, nil
	}
	var res struct {
		Results            []page
		Start, Size, Limit int
		Links              struct{ Next string } `json:"_links"`
	}
	if e = request(ctx, transport, "GET", endpoint, nil, headers, &res); e != nil {
		return out, e
	}
	for _, v := range res.Results {
		out.Items = append(out.Items, convert(v))
	}
	if res.Links.Next != "" && len(res.Results) > 0 {
		out.Next = strconv.Itoa(start + len(res.Results))
	}
	return out, nil
}

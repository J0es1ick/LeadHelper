package integrations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"leadhelper/internal/domain"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Provider instances are immutable snapshots. Reconnection never races a worker.
type Hub struct {
	mu      sync.RWMutex
	configs map[string]domain.IntegrationConfig
	Save    func([]byte) error
	Client  *http.Client
}

func NewHub(configs map[string]domain.IntegrationConfig, persisted []byte, save func([]byte) error) (*Hub, error) {
	if configs == nil {
		configs = map[string]domain.IntegrationConfig{}
	}
	if len(persisted) > 0 {
		var stored map[string]domain.IntegrationConfig
		if e := json.Unmarshal(persisted, &stored); e != nil {
			return nil, errors.New("Повреждено хранилище подключений")
		}
		for k, v := range stored {
			configs[k] = v
		}
	}
	return &Hub{configs: configs, Save: save}, nil
}
func (h *Hub) config(id string) domain.IntegrationConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.configs[id]
}
func (h *Hub) PublicConfig(id string) domain.IntegrationConfig {
	c := h.config(id)
	c.Token = ""
	if id == "bitrix" {
		u, e := url.Parse(c.URL)
		if e == nil && u.Host != "" {
			c.URL = u.Scheme + "://" + u.Host
		} else {
			c.URL = ""
		}
	}
	return c
}
func (h *Hub) jira(c domain.IntegrationConfig) *Jira {
	return &Jira{URL: c.URL, Email: c.Email, Token: c.Token, JQL: c.JQL, Mode: c.Mode, Scope: c.Scope, Client: h.Client}
}
func (h *Hub) bitrix() *Bitrix {
	c := h.config("bitrix")
	return &Bitrix{Webhook: c.URL, Client: h.Client}
}
func (h *Hub) confluence() *Confluence {
	c := h.config("confluence")
	return &Confluence{URL: c.URL, Token: c.Token, Client: h.Client}
}
func (h *Hub) Configure(ctx context.Context, id string, c domain.IntegrationConfig) error {
	if id != "jira" && id != "bitrix" && id != "confluence" && id != "google-sheets" {
		return errors.New("Неизвестная интеграция")
	}
	old := h.config(id)
	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	c.Token = strings.TrimSpace(c.Token)
	c.Email = strings.TrimSpace(c.Email)
	if strings.ContainsAny(c.Token+c.Email, "\r\n") || len(c.Token) > 4096 || len(c.URL) > 2048 || len(c.JQL) > 4000 {
		return errors.New("Некорректные параметры подключения")
	}
	if id == "bitrix" {
		if c.Token != "" {
			c.URL = c.Token
			c.Token = ""
		} else {
			c.URL = old.URL
		}
	} else if c.Token == "" {
		if c.URL != strings.TrimRight(old.URL, "/") {
			return errors.New("Для нового сервера введите новый токен")
		}
		c.Token = old.Token
	}
	if e := httpsURL(c.URL); e != nil {
		return e
	}
	transport := h.Client
	if transport == nil {
		transport = client()
	}
	switch id {
	case "google-sheets":
		if _, e := (ProjectSheets{Config: c, Client: h.Client}).Snapshot(ctx); e != nil {
			return e
		}
	case "jira":
		if c.Scope == "" {
			c.Scope = "team"
		}
		if c.Scope != "team" && c.Scope != "jql" {
			return errors.New("Неизвестный режим загрузки Jira")
		}
		if c.Scope == "jql" && strings.TrimSpace(c.JQL) == "" {
			return errors.New("Укажите JQL или выберите задачи команды")
		}
		if c.Token == "" {
			return errors.New("Введите персональный токен Jira")
		}
		if c.Mode != "server" && c.Mode != "cloud" {
			c.Mode = "server"
		}
		j := h.jira(c)
		path := "/rest/api/2/myself"
		auth := "Bearer " + c.Token
		if j.cloud() {
			if c.Email == "" {
				return errors.New("Для Jira Cloud нужен email")
			}
			path = "/rest/api/3/myself"
			auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(c.Email+":"+c.Token))
		}
		var profile map[string]any
		if e := request(ctx, transport, "GET", c.URL+path, nil, map[string]string{"Authorization": auth}, &profile); e != nil {
			return e
		}
	case "bitrix":
		b := &Bitrix{Webhook: c.URL, Client: h.Client}
		if _, e := b.identity(ctx); e != nil {
			return e
		}
	case "confluence":
		p := &Confluence{URL: c.URL, Token: c.Token, Client: h.Client}
		if _, e := p.Browse(ctx, domain.RemoteQuery{Kind: "spaces"}); e != nil {
			return e
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	updated := map[string]domain.IntegrationConfig{}
	for k, v := range h.configs {
		updated[k] = v
	}
	updated[id] = c
	data, e := json.Marshal(updated)
	if e != nil {
		return e
	}
	if h.Save == nil {
		return errors.New("Хранилище подключений недоступно")
	}
	if e = h.Save(data); e != nil {
		return e
	}
	h.configs = updated
	return nil
}

type LiveJira struct{ Hub *Hub }

func (p LiveJira) Configured() bool { return p.Hub.jira(p.Hub.config("jira")).Configured() }
func (p LiveJira) FetchTasks(ctx context.Context) ([]domain.Task, []domain.Member, error) {
	return p.Hub.jira(p.Hub.config("jira")).FetchTasks(ctx)
}
func (p LiveJira) TeamScoped() bool { return p.Hub.jira(p.Hub.config("jira")).TeamScoped() }
func (p LiveJira) SiteID() string   { return p.Hub.jira(p.Hub.config("jira")).SiteID() }
func (p LiveJira) SearchUsers(ctx context.Context, search string) ([]domain.JiraUser, error) {
	return p.Hub.jira(p.Hub.config("jira")).SearchUsers(ctx, search)
}
func (p LiveJira) User(ctx context.Context, id string) (domain.JiraUser, error) {
	return p.Hub.jira(p.Hub.config("jira")).User(ctx, id)
}
func (p LiveJira) FetchTeamTasks(ctx context.Context, members []domain.Member) ([]domain.Task, error) {
	return p.Hub.jira(p.Hub.config("jira")).FetchTeamTasks(ctx, members)
}

type LiveBitrix struct{ Hub *Hub }

func (p LiveBitrix) Configured() bool  { return p.Hub.bitrix().Configured() }
func (p LiveBitrix) AccountID() string { return p.Hub.bitrix().AccountID() }
func (p LiveBitrix) Browse(ctx context.Context, q domain.RemoteQuery) (domain.RemotePage, error) {
	return p.Hub.bitrix().Browse(ctx, q)
}
func (p LiveBitrix) FetchTeam(ctx context.Context) (domain.TeamSnapshot, error) {
	return p.Hub.bitrix().FetchTeam(ctx)
}
func (p LiveBitrix) Calendar(ctx context.Context, from, to string) (domain.CalendarPage, error) {
	return p.Hub.bitrix().Calendar(ctx, from, to)
}
func (p LiveBitrix) SendChat(ctx context.Context, id, text string) error {
	return p.Hub.bitrix().SendChat(ctx, id, text)
}
func (p LiveBitrix) ReadChat(ctx context.Context, id, messageID string) (domain.ChatRead, error) {
	return p.Hub.bitrix().ReadChat(ctx, id, messageID)
}

type LiveConfluence struct{ Hub *Hub }

func (p LiveConfluence) Configured() bool { return p.Hub.confluence().Configured() }
func (p LiveConfluence) Browse(ctx context.Context, q domain.RemoteQuery) (domain.RemotePage, error) {
	return p.Hub.confluence().Browse(ctx, q)
}

package integrations

import (
	"context"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

type ProjectSheets struct {
	Config domain.IntegrationConfig
	Client *http.Client
}

var scriptPath = regexp.MustCompile(`^/macros/s/[A-Za-z0-9_-]+/exec$`)

func (p ProjectSheets) Configured() bool { return p.Config.URL != "" && p.Config.Token != "" }
func (p ProjectSheets) call(ctx context.Context, action string, change *domain.ProjectSheetChange) (domain.ProjectSheetSnapshot, error) {
	var out struct {
		OK       bool                        `json:"ok"`
		Error    string                      `json:"error"`
		Snapshot domain.ProjectSheetSnapshot `json:"snapshot"`
	}
	u, e := url.Parse(p.Config.URL)
	if e != nil || u.Scheme != "https" || u.Host != "script.google.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !scriptPath.MatchString(u.Path) || len(p.Config.Token) < 32 {
		return out.Snapshot, errors.New("Укажите URL веб-приложения Apps Script /exec и секрет подключения")
	}
	c := p.Client
	if c == nil {
		c = client()
	}
	copy := *c
	copy.Timeout = 45 * time.Second
	copy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.Host != "script.googleusercontent.com" || req.URL.User != nil {
			return errors.New("Неожиданное перенаправление Google")
		}
		return nil
	}
	body := map[string]any{"token": p.Config.Token, "action": action}
	if change != nil {
		body["change"] = change
	}
	if e = request(ctx, &copy, "POST", p.Config.URL, body, nil, &out); e != nil {
		return out.Snapshot, fmt.Errorf("Google Таблицы: подключение недоступно. Проверьте URL, доступ веб-приложения и секрет")
	}
	if !out.OK {
		if len(out.Error) > 500 {
			out.Error = "Ошибка синхронизации"
		}
		return out.Snapshot, fmt.Errorf("Google Таблицы: %s", out.Error)
	}
	if action == "snapshot" && (out.Snapshot.SpreadsheetID == "" || len(out.Snapshot.SpreadsheetID) > 200 || out.Snapshot.Title == "" || len(out.Snapshot.Sheets) > 40) {
		return out.Snapshot, errors.New("Google Таблицы: скрипт не вернул корректный снимок книги")
	}
	return out.Snapshot, nil
}
func (p ProjectSheets) Snapshot(ctx context.Context) (domain.ProjectSheetSnapshot, error) {
	return p.call(ctx, "snapshot", nil)
}
func (p ProjectSheets) Apply(ctx context.Context, c domain.ProjectSheetChange) error {
	_, e := p.call(ctx, "apply", &c)
	return e
}

type LiveProjectSheets struct{ Hub *Hub }

func (p LiveProjectSheets) source() ProjectSheets {
	return ProjectSheets{Config: p.Hub.config("google-sheets"), Client: p.Hub.Client}
}
func (p LiveProjectSheets) Configured() bool { return p.source().Configured() }
func (p LiveProjectSheets) Snapshot(ctx context.Context) (domain.ProjectSheetSnapshot, error) {
	return p.source().Snapshot(ctx)
}
func (p LiveProjectSheets) Apply(ctx context.Context, c domain.ProjectSheetChange) error {
	return p.source().Apply(ctx, c)
}

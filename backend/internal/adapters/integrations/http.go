package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Provider credentials are server-only environment configuration. Redirects are
// disabled so tokens and webhook paths cannot leak to another origin.
func client() *http.Client {
	return &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func httpsURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return errors.New("Для интеграции нужен корректный HTTPS URL без userinfo и fragment")
	}
	return nil
}
func request(ctx context.Context, c *http.Client, method, endpoint string, body any, headers map[string]string, out any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
	if err != nil {
		return errors.New("Некорректная конфигурация интеграции")
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	resp, err := c.Do(r)
	if err != nil {
		return errors.New("Не удалось связаться с сервисом: проверьте адрес, сеть и срок действия токена")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Сервис вернул HTTP %d. Проверьте доступ и настройки интеграции", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 20<<20)).Decode(out); err != nil {
		return errors.New("Некорректный или слишком большой ответ сервиса")
	}
	return nil
}
func date(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return ""
}

func providerTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05-0700"} {
		if v, e := time.Parse(layout, s); e == nil {
			return v
		}
	}
	return time.Time{}
}

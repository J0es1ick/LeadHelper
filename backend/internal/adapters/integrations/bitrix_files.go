package integrations

import (
	"context"
	"fmt"
	"io"
	"leadhelper/internal/domain"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var bitrixFileID = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
var bitrixFileTag = regexp.MustCompile(`(?i)\[DISK FILE ID=([0-9]+)\]`)

func messageFileText(text string, files []domain.RemoteAttachment) string {
	known := map[string]bool{}
	for _, f := range files {
		known[f.ID] = true
	}
	return strings.TrimSpace(bitrixFileTag.ReplaceAllStringFunc(text, func(tag string) string {
		m := bitrixFileTag.FindStringSubmatch(tag)
		if known[m[1]] {
			return ""
		}
		return tag
	}))
}

func (b *Bitrix) messageFiles(message map[string]any, files []map[string]any) []domain.RemoteAttachment {
	ids := map[string]bool{}
	params := obj(message["params"])
	v := params["FILE_ID"]
	if v == nil {
		v = params["fileId"]
	}
	switch x := v.(type) {
	case []any:
		for _, id := range x {
			ids[str(id)] = true
		}
	case string:
		for _, id := range strings.Split(x, ",") {
			ids[strings.TrimSpace(id)] = true
		}
	default:
		ids[str(v)] = true
	}
	out := []domain.RemoteAttachment{}
	seen := map[string]bool{}
	for _, f := range files {
		id := str(f["id"])
		if !bitrixFileID.MatchString(id) || !ids[id] || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, b.fileInfo(id, str(f["name"]), int64(integer(f["size"]))))
	}
	return out
}
func (b *Bitrix) fileInfo(id, name string, size int64) domain.RemoteAttachment {
	ext := strings.ToLower(filepath.Ext(name))
	image := ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" || ext == ".bmp"
	return domain.RemoteAttachment{ID: id, Name: name, Size: size, Image: image, URL: "/api/bitrix/files/" + id + "?account=" + url.QueryEscape(b.AccountID())}
}
func (b *Bitrix) downloadURL(raw string) (*url.URL, error) {
	base, _ := url.Parse(b.base())
	u, e := url.Parse(raw)
	if e != nil || base == nil {
		return nil, fmt.Errorf("Некорректная ссылка файла")
	}
	u = base.ResolveReference(u)
	host := strings.ToLower(u.Hostname())
	allowed := u.Host == base.Host
	for _, suffix := range []string{".bitrix24.ru", ".bitrix24.com", ".bitrix24.net", ".bitrix24-cdn.com", ".bitrix.info"} {
		allowed = allowed || strings.HasSuffix(host, suffix)
	}
	if !allowed || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Host != base.Host && u.Port() != "" && u.Port() != "443" {
		return nil, fmt.Errorf("Хранилище файла недоступно через интеграцию; откройте файл в Битрикс24")
	}
	return u, nil
}
func (b *Bitrix) DownloadFile(ctx context.Context, id string) (io.ReadCloser, string, string, int64, error) {
	if !bitrixFileID.MatchString(id) {
		return nil, "", "", 0, fmt.Errorf("Некорректный файл")
	}
	var file struct {
		NAME         string
		SIZE         any
		DOWNLOAD_URL string
	}
	if _, e := b.call(ctx, "disk.file.get", map[string]any{"id": id}, &file); e != nil {
		return nil, "", "", 0, e
	}
	if file.DOWNLOAD_URL == "" {
		return nil, "", "", 0, fmt.Errorf("Битрикс24 не предоставил ссылку. Проверьте право «Диск» у вебхука")
	}
	u, e := b.downloadURL(file.DOWNLOAD_URL)
	if e != nil {
		return nil, "", "", 0, e
	}
	c := b.Client
	if c == nil {
		c = client()
	}
	copy := *c
	copy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return fmt.Errorf("Слишком много перенаправлений")
		}
		_, e := b.downloadURL(req.URL.String())
		return e
	}
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return nil, "", "", 0, fmt.Errorf("Не удалось открыть файл")
	}
	res, e := copy.Do(req)
	if e != nil {
		return nil, "", "", 0, fmt.Errorf("Не удалось скачать файл из Битрикс24")
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, "", "", 0, fmt.Errorf("Битрикс24 не разрешил скачивание файла (HTTP %d)", res.StatusCode)
	}
	return res.Body, file.NAME, res.Header.Get("Content-Type"), res.ContentLength, nil
}
func (p LiveBitrix) DownloadFile(ctx context.Context, id string) (io.ReadCloser, string, string, int64, error) {
	return p.Hub.bitrix().DownloadFile(ctx, id)
}

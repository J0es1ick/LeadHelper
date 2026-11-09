package httpapi

import (
	"bytes"
	"io"
	"leadhelper/internal/adapters/filepreview"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type remoteFile struct {
	name, contentType string
	data              []byte
	expires           time.Time
}

// A brief, bounded cache lets PDF byte ranges and preview metadata share one
// download. Every request is authorized again; signed provider URLs are never cached.
type remoteFileCache struct {
	mu    sync.Mutex
	files map[string]remoteFile
	slots chan struct{}
}

func (c *remoteFileCache) get(key string) (remoteFile, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range c.files {
		if time.Now().After(v.expires) {
			delete(c.files, k)
		}
	}
	v, ok := c.files[key]
	return v, ok
}
func (c *remoteFileCache) put(key string, v remoteFile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	const maxBytes = 64 << 20
	delete(c.files, key)
	total := len(v.data)
	for _, f := range c.files {
		total += len(f.data)
	}
	for total > maxBytes || len(c.files) >= 64 {
		var oldestKey string
		var oldest time.Time
		for k, f := range c.files {
			if oldestKey == "" || f.expires.Before(oldest) {
				oldestKey = k
				oldest = f.expires
			}
		}
		if oldestKey == "" {
			break
		}
		total -= len(c.files[oldestKey].data)
		delete(c.files, oldestKey)
	}
	c.files[key] = v
}
func (a *API) bitrixFileBytes(w http.ResponseWriter, r *http.Request) (remoteFile, bool) {
	account := r.URL.Query().Get("account")
	if e := a.s.CheckBitrixFile(r.Context(), account); e != nil {
		respond(w, nil, e)
		return remoteFile{}, false
	}
	key := account + ":" + r.PathValue("id")
	if v, ok := a.remoteFiles.get(key); ok {
		return v, true
	}
	select {
	case a.remoteFiles.slots <- struct{}{}:
		defer func() { <-a.remoteFiles.slots }()
	case <-r.Context().Done():
		return remoteFile{}, false
	}
	if v, ok := a.remoteFiles.get(key); ok {
		return v, true
	}
	body, name, contentType, size, e := a.s.BitrixFile(r.Context(), r.PathValue("id"), account)
	if e != nil {
		respond(w, nil, e)
		return remoteFile{}, false
	}
	defer body.Close()
	const limit = 50 << 20
	if size > limit {
		send(w, 413, map[string]string{"error": "Файл больше 50 МБ. Откройте его в Битрикс24"})
		return remoteFile{}, false
	}
	data, e := io.ReadAll(io.LimitReader(body, limit+1))
	if e != nil {
		send(w, 502, map[string]string{"error": "Не удалось полностью скачать файл"})
		return remoteFile{}, false
	}
	if len(data) > limit {
		send(w, 413, map[string]string{"error": "Файл больше 50 МБ"})
		return remoteFile{}, false
	}
	if e = a.s.CheckBitrixFile(r.Context(), account); e != nil {
		respond(w, nil, e)
		return remoteFile{}, false
	}
	name = strings.ReplaceAll(strings.ReplaceAll(name, "\\", "_"), "/", "_")
	if name == "" {
		name = "file"
	}
	file := remoteFile{name: name, contentType: contentType, data: data, expires: time.Now().Add(time.Minute)}
	a.remoteFiles.put(key, file)
	return file, true
}
func (a *API) bitrixFilePreview(w http.ResponseWriter, r *http.Request) {
	file, ok := a.bitrixFileBytes(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	send(w, 200, filepreview.Build(file.name, file.contentType, file.data))
}
func (a *API) bitrixFileContent(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("format") == "docx" {
		file, ok := a.bitrixFileBytes(w, r)
		if !ok {
			return
		}
		if !strings.EqualFold(filepath.Ext(file.name), ".docx") || filepreview.ValidateDOCX(file.data) != nil {
			send(w, 415, map[string]string{"error": "Документ повреждён или слишком сложен для предпросмотра"})
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		w.Header().Set("Content-Disposition", "attachment")
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeContent(w, r, file.name, time.Time{}, bytes.NewReader(file.data))
		return
	}
	a.serveBitrixFile(w, r, true)
}
func (a *API) bitrixFile(w http.ResponseWriter, r *http.Request) {
	a.serveBitrixFile(w, r, r.URL.Query().Get("preview") == "1")
}
func (a *API) serveBitrixFile(w http.ResponseWriter, r *http.Request, inline bool) {
	file, ok := a.bitrixFileBytes(w, r)
	if !ok {
		return
	}
	kind := http.DetectContentType(file.data)
	disposition := "attachment"
	if inline {
		kind = filepreview.InlineType(file.data)
		if kind == "" {
			send(w, 415, map[string]string{"error": "Этот формат нельзя открыть как изображение или PDF"})
			return
		}
		disposition = "inline"
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.name}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, file.name, time.Time{}, bytes.NewReader(file.data))
}

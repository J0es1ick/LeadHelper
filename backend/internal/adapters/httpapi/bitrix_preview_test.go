package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leadhelper/internal/adapters/filepreview"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/application"
	"leadhelper/internal/domain"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type previewBitrixFiles struct{ calls int }

func (*previewBitrixFiles) Configured() bool  { return true }
func (*previewBitrixFiles) AccountID() string { return "files-account" }
func (*previewBitrixFiles) Browse(context.Context, domain.RemoteQuery) (domain.RemotePage, error) {
	return domain.RemotePage{}, nil
}
func (*previewBitrixFiles) Calendar(context.Context, string, string) (domain.CalendarPage, error) {
	return domain.CalendarPage{}, nil
}
func (*previewBitrixFiles) SendChat(context.Context, string, string) error {
	return fmt.Errorf("not implemented")
}
func (f *previewBitrixFiles) DownloadFile(_ context.Context, id string) (io.ReadCloser, string, string, int64, error) {
	f.calls++
	var name, kind string
	var data []byte
	switch id {
	case "1":
		name = "sample.pdf"
		kind = "application/pdf"
		data = []byte("%PDF-1.4\nfixture")
	case "2":
		name = "notes.txt"
		kind = "text/plain"
		data = []byte("Файл из Битрикса")
	case "3":
		name = "fake.pdf"
		kind = "application/pdf"
		data = []byte("<script>bad()</script>")
	case "4":
		name = "brief.docx"
		kind = "application/octet-stream"
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		p, _ := z.Create("word/document.xml")
		p.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Проверка DOCX</w:t></w:r></w:p></w:body></w:document>`))
		z.Close()
		data = b.Bytes()
	case "5":
		name = "large.pdf"
		return io.NopCloser(strings.NewReader("")), name, "application/pdf", 51 << 20, nil
	default:
		return nil, "", "", 0, fmt.Errorf("Файл недоступен")
	}
	return io.NopCloser(bytes.NewReader(data)), name, kind, int64(len(data)), nil
}
func TestBitrixPreviewTypesRangesCacheAndCurrentAccess(t *testing.T) {
	st := application.Seed(time.Now())
	st.Settings.Demo = false
	r, e := storage.Open(filepath.Join(t.TempDir(), "state.json"), st)
	if e != nil {
		t.Fatal(e)
	}
	f := &previewBitrixFiles{}
	s := application.New(r, nil, nil).WithWorkspace(f, nil, nil)
	h := New(s, t.TempDir())
	get := func(route, account, byteRange string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://localhost:8090/api/bitrix/files/"+route, nil)
		query := req.URL.Query()
		query.Set("account", account)
		req.URL.RawQuery = query.Encode()
		req.Header.Set("Range", byteRange)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	for _, tc := range []struct{ route, kind, text string }{{"1/preview", "pdf", ""}, {"2/preview", "text", "Файл из Битрикса"}, {"4/preview", "docx", "Проверка DOCX"}} {
		w := get(tc.route, "files-account", "")
		var p filepreview.Preview
		if e = json.Unmarshal(w.Body.Bytes(), &p); e != nil || w.Code != 200 || p.Kind != tc.kind || !strings.Contains(p.Text, tc.text) {
			t.Fatal(tc, w.Code, w.Body.String(), e)
		}
	}
	w := get("1/preview/content", "files-account", "bytes=0-4")
	if w.Code != 206 || w.Body.String() != "%PDF-" || w.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline;") || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("PDF range", w.Code, w.Header(), w.Body.String())
	}
	if f.calls != 3 {
		t.Fatal("PDF content refetched after preview", f.calls)
	}
	w = get("1", "files-account", "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") || f.calls != 3 {
		t.Fatal("download or cache changed", w.Code, f.calls)
	}
	if w = get("3/preview/content", "files-account", ""); w.Code != 415 {
		t.Fatal("active HTML inline", w.Code)
	}
	w = get("4/preview/content?format=docx", "files-account", "bytes=0-3")
	if w.Code != 206 || w.Body.Len() != 4 || !strings.Contains(w.Header().Get("Content-Type"), "wordprocessingml") || w.Header().Get("Content-Disposition") != "attachment" {
		t.Fatal("DOCX preview bytes", w.Code, w.Header())
	}
	if w = get("3/preview/content?format=docx", "files-account", ""); w.Code != 415 {
		t.Fatal("non-DOCX accepted", w.Code)
	}
	if w = get("5/preview", "files-account", ""); w.Code != 413 {
		t.Fatal("large file accepted", w.Code)
	}
	if w = get("1/preview", "different-account", ""); w.Code != 400 {
		t.Fatal("other connection cache visible", w.Code)
	}
	if e = r.Update(context.Background(), func(s *domain.State) error { s.Settings.Modules["bitrix"] = false; return nil }); e != nil {
		t.Fatal(e)
	}
	if w = get("1/preview/content", "files-account", ""); w.Code != 403 {
		t.Fatal("disabled module cache visible", w.Code)
	}
	if e = r.Update(context.Background(), func(s *domain.State) error { s.Settings.Modules["bitrix"] = true; s.Settings.Demo = true; return nil }); e != nil {
		t.Fatal(e)
	}
	if w = get("1/preview", "files-account", ""); w.Code != 400 {
		t.Fatal("production cache exposed in demo", w.Code)
	}
}
func TestBitrixPreviewCacheExpiryAndBounds(t *testing.T) {
	c := remoteFileCache{files: map[string]remoteFile{}}
	c.put("expired", remoteFile{data: []byte("x"), expires: time.Now().Add(-time.Second)})
	if _, ok := c.get("expired"); ok {
		t.Fatal("expired entry returned")
	}
	for i := 0; i < 100; i++ {
		c.put(fmt.Sprint(i), remoteFile{data: []byte("x"), expires: time.Now().Add(time.Minute)})
	}
	if len(c.files) > 64 {
		t.Fatal("too many cached files")
	}
	chunk := make([]byte, 33<<20)
	c.put("large-a", remoteFile{data: chunk, expires: time.Now().Add(time.Minute)})
	c.put("large-b", remoteFile{data: chunk, expires: time.Now().Add(time.Minute)})
	size := 0
	for _, v := range c.files {
		size += len(v.data)
	}
	if size > 64<<20 {
		t.Fatal("cache unbounded", size)
	}
}

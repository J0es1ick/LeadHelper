package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"leadhelper/internal/adapters/filepreview"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/application"
	"leadhelper/internal/domain"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAttachmentPreviewAccessTypesAndRanges(t *testing.T) {
	var doc bytes.Buffer
	z := zip.NewWriter(&doc)
	part, _ := z.Create("word/document.xml")
	part.Write([]byte(`<document><body><p>Test</p></body></document>`))
	z.Close()
	seed := application.Seed(time.Now())
	seed.Messages = []domain.Message{{ID: "preview", Demo: true, Attachments: []domain.Attachment{
		{ID: "text", Name: "note.txt", ContentType: "text/plain", Data: base64.StdEncoding.EncodeToString([]byte("Привет"))},
		{ID: "pdf", Name: "sample.pdf", ContentType: "application/pdf", Data: base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\nfixture"))},
		{ID: "fake", Name: "fake.pdf", ContentType: "application/pdf", Data: base64.StdEncoding.EncodeToString([]byte("<script>bad()</script>"))},
		{ID: "docx", Name: "test.docx", Data: base64.StdEncoding.EncodeToString(doc.Bytes())},
		{ID: "bad-docx", Name: "bad.docx", Data: base64.StdEncoding.EncodeToString([]byte("not a document"))},
	}}}
	repo, err := storage.Open(filepath.Join(t.TempDir(), "db.json"), seed)
	if err != nil {
		t.Fatal(err)
	}
	s := application.New(repo, nil, nil)
	h := New(s, t.TempDir())
	get := func(route, byteRange string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://localhost:8090/api/mail/preview/attachments/"+route, nil)
		r.Header.Set("Range", byteRange)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := get("text/preview", "")
	var preview filepreview.Preview
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil || w.Code != 200 || preview.Text != "Привет" {
		t.Fatalf("text preview failed: %d %s", w.Code, w.Body.String())
	}
	w = get("pdf/preview/content", "bytes=0-4")
	if w.Code != 206 || w.Body.String() != "%PDF-" || w.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline;") || w.Header().Get("X-Frame-Options") != "SAMEORIGIN" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("PDF headers/range: %d %+v", w.Code, w.Header())
	}
	if w := get("fake/preview/content", ""); w.Code != 415 {
		t.Fatal("active HTML served inline")
	}
	if w := get("missing/preview", ""); w.Code != 404 {
		t.Fatal("missing attachment exposed")
	}
	w = get("docx/preview/content?format=docx", "bytes=0-3")
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), doc.Bytes()[:4]) || !strings.Contains(w.Header().Get("Content-Type"), "wordprocessingml") || w.Header().Get("Content-Disposition") != "attachment" {
		t.Fatal("DOCX preview bytes or headers", w.Code, w.Header())
	}
	for _, route := range []string{"bad-docx/preview/content?format=docx", "pdf/preview/content?format=docx", "docx/preview/content"} {
		if w := get(route, ""); w.Code != 415 {
			t.Fatal("invalid format accepted", route, w.Code)
		}
	}
	if w := get("pdf", ""); w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("download behavior changed")
	}
	repo.Update(context.Background(), func(st *domain.State) error { st.Settings.Demo = false; return nil })
	for _, route := range []string{"text/preview", "pdf/preview/content", "docx/preview/content?format=docx"} {
		if w := get(route, ""); w.Code != 404 {
			t.Fatal("other mode attachment exposed")
		}
	}
	repo.Update(context.Background(), func(st *domain.State) error { st.Settings.Demo = true; st.Settings.Modules["mail"] = false; return nil })
	if w := get("text/preview", ""); w.Code != 403 {
		t.Fatalf("disabled mail exposed: %d", w.Code)
	}
}

func TestPDFRenderingAssetTypes(t *testing.T) {
	dir := t.TempDir()
	for _, file := range []struct{ name, contentType string }{{"worker.mjs", "text/javascript; charset=utf-8"}, {"decoder.wasm", "application/wasm"}} {
		if err := os.WriteFile(filepath.Join(dir, file.name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("GET", "http://localhost:8090/"+file.name, nil)
		w := httptest.NewRecorder()
		New(nil, dir).ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Content-Type") != file.contentType {
			t.Fatalf("asset %s has wrong MIME type: %d %+v", file.name, w.Code, w.Header())
		}
	}
}

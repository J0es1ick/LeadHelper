package httpapi

import (
	"bytes"
	"context"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/application"
	"leadhelper/internal/domain"
	"mime/multipart"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

type httpUploadSource struct {
	calls int
	file  domain.ChatUpload
}

func (*httpUploadSource) Configured() bool  { return true }
func (*httpUploadSource) AccountID() string { return "qa-account" }
func (*httpUploadSource) Browse(context.Context, domain.RemoteQuery) (domain.RemotePage, error) {
	return domain.RemotePage{}, nil
}
func (*httpUploadSource) Calendar(context.Context, string, string) (domain.CalendarPage, error) {
	return domain.CalendarPage{}, nil
}
func (*httpUploadSource) SendChat(context.Context, string, string) error { return nil }
func (s *httpUploadSource) SendChatFile(_ context.Context, _ string, file domain.ChatUpload) (domain.ChatUploadResult, error) {
	s.calls++
	s.file = file
	return domain.ChatUploadResult{MessageID: "99", FileID: "12"}, nil
}
func TestChatMultipartUploadValidatesAndDeduplicates(t *testing.T) {
	seed := application.Seed(time.Now())
	seed.Settings.Demo = false
	seed.Settings.Modules["bitrix"] = true
	repo, e := storage.Open(filepath.Join(t.TempDir(), "db.json"), seed)
	if e != nil {
		t.Fatal(e)
	}
	source := &httpUploadSource{}
	service := application.New(repo, nil, nil).WithWorkspace(source, nil, nil)
	handler := New(service, t.TempDir())
	for _, tc := range []struct {
		account, request string
		files            int
		code             int
	}{{"qa-account", "request-00000001", 1, 200}, {"qa-account", "request-00000001", 1, 200}, {"wrong", "request-00000002", 1, 400}, {"qa-account", "request-00000003", 2, 400}} {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		_ = form.WriteField("accountId", tc.account)
		_ = form.WriteField("requestId", tc.request)
		_ = form.WriteField("text", "Подпись")
		for i := 0; i < tc.files; i++ {
			file, _ := form.CreateFormFile("file", "отчёт.txt")
			_, _ = file.Write([]byte("данные"))
		}
		_ = form.Close()
		r := httptest.NewRequest("POST", "http://localhost:8090/api/bitrix/chats/chat42/files", &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("X-LeadHelper", "1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("got %d want %d: %s", w.Code, tc.code, w.Body.String())
		}
	}
	if source.calls != 1 || source.file.Name != "отчёт.txt" || source.file.Text != "Подпись" || string(source.file.Data) != "данные" {
		t.Fatal(source.calls, source.file.Name, source.file.Text)
	}
}

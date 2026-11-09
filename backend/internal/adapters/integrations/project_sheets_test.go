package integrations

import (
	"context"
	"encoding/json"
	"io"
	"leadhelper/internal/domain"
	"net/http"
	"strings"
	"testing"
)

type sheetsTransport func(*http.Request) (*http.Response, error)

func (f sheetsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestProjectSheetsAuthAndSnapshotValidation(t *testing.T) {
	calls := 0
	response := `{"ok":true,"snapshot":{"spreadsheetId":"book","title":"Projects","sheets":[]}}`
	p := ProjectSheets{Config: domain.IntegrationConfig{URL: "https://script.google.com/macros/s/deployment/exec", Token: strings.Repeat("x", 64)}, Client: &http.Client{Transport: sheetsTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Fatal(e)
		}
		if body["token"] != strings.Repeat("x", 64) || body["action"] != "snapshot" || r.URL.RawQuery != "" {
			t.Fatal("auth transport changed")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: http.Header{}, Request: r}, nil
	})}}
	book, e := p.Snapshot(context.Background())
	if e != nil || book.SpreadsheetID != "book" {
		t.Fatal(book, e)
	}
	response = `{"ok":true}`
	if _, e = p.Snapshot(context.Background()); e == nil {
		t.Fatal("empty snapshot accepted")
	}
	p.Config.URL = "https://evil.test/macros/s/deployment/exec"
	if _, e = p.Snapshot(context.Background()); e == nil || calls != 2 {
		t.Fatal("invalid host reached network")
	}
}

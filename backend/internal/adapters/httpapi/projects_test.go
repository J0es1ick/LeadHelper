package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"leadhelper/internal/adapters/filepreview"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/application"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProjectImportRejectsTruncationAndInvalidEncoding(t *testing.T) {
	repo, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "projects.db"), "", application.Seed(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	h := New(application.New(repo, nil, nil), t.TempDir())
	parse := func(name string, data []byte) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]string{"name": name, "data": base64.StdEncoding.EncodeToString(data)})
		req := httptest.NewRequest("POST", "http://localhost:8090/api/projects/import/parse", bytes.NewReader(raw))
		req.Header.Set("X-LeadHelper", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	w := parse("projects.csv", []byte("Проект;Статус\nПлатформа;В работе"))
	var p filepreview.Preview
	if err = json.Unmarshal(w.Body.Bytes(), &p); err != nil || w.Code != 200 || p.Kind != "table" || p.Sheets[0].Rows[1][0] != "Платформа" {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"too-many.csv", []byte(strings.Repeat("project,status\n", 5001))},
		{"columns.csv", []byte(strings.Repeat("column;", 401))},
		{"encoding.csv", []byte{0xef, 0xf0, 0xee, 0xe5, 0xea, 0xf2}},
		{"bad.xlsx", []byte("not a zip")},
		{"wrong.html", []byte("<html>test</html>")},
	} {
		if w = parse(tc.name, tc.data); w.Code != 400 {
			t.Fatal(tc.name, w.Code, w.Body.String())
		}
	}
	req := httptest.NewRequest("GET", "http://localhost:8090/api/projects", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":0`) {
		t.Fatal("parsing wrote projects", w.Code, w.Body.String())
	}
}

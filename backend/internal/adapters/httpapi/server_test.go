package httpapi

import (
	"context"
	"encoding/json"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/application"
	"leadhelper/internal/domain"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPBoundary(t *testing.T) {
	repo, e := storage.Open(filepath.Join(t.TempDir(), "db.json"), application.Seed(time.Now()))
	if e != nil {
		t.Fatal(e)
	}
	s := application.New(repo, nil, nil)
	handler := New(s, t.TempDir())
	for _, tc := range []struct {
		host, origin, header, body string
		code                       int
	}{{"localhost:8090", "", "", `{}`, 403}, {"attacker.example", "", "1", `{}`, 403}, {"localhost:8090", "https://evil.example", "1", `{}`, 403}, {"localhost:8090", "http://localhost:8090", "1", `{"title":"created","status":"todo","priority":"high"}`, 200}, {"localhost:8090", "", "1", `{"title":"x","unknown":1}`, 400}, {"localhost:8090", "", "1", `{} {}`, 400}} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/api/tasks", strings.NewReader(tc.body))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-LeadHelper", tc.header)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("want %d got %d: %s", tc.code, w.Code, w.Body.String())
		}
	}
	st, _ := s.State(context.Background())
	cfg := st.Settings
	cfg.Modules["mail"] = false
	_ = s.Settings(context.Background(), cfg)
	r := httptest.NewRequest("GET", "http://localhost:8090/api/state", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	var got domain.State
	if e = json.Unmarshal(w.Body.Bytes(), &got); e != nil {
		t.Fatal(e)
	}
	if len(got.Messages) != 0 {
		t.Fatal("disabled mail exposed")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing private cache protection")
	}
}
func TestCSVFormulaEscaping(t *testing.T) {
	for _, s := range []string{"=1+1", " +cmd", "@SUM(1)", "-2", "\t=evil"} {
		if !strings.HasPrefix(safeCell(s), "'") {
			t.Fatalf("unsafe cell %q", s)
		}
	}
	if safeCell("") != "" || safeCell("normal") != "normal" {
		t.Fatal("normal cell corrupted")
	}
}

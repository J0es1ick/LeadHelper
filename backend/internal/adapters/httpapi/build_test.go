package httpapi

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFrontendBuildAndHTMLRevalidation(t *testing.T) {
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "index.html"), []byte(`<script type="module" src="/assets/index-new.js"></script>`), 0600); e != nil {
		t.Fatal(e)
	}
	if got := frontendBuild(dir); got != "/assets/index-new.js" {
		t.Fatal("wrong build", got)
	}
	a := &API{static: dir}
	for _, path := range []string{"/", "/index.html", "/mail"} {
		w := httptest.NewRecorder()
		a.spa(w, httptest.NewRequest("GET", path, nil))
		if (w.Code != 200 && !(path == "/index.html" && w.Code == 301)) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("HTML may stay stale", path, w.Code, w.Header())
		}
	}
	os.WriteFile(filepath.Join(dir, "index.html"), []byte(`<script src="/assets/index-newer.js"></script>`), 0600)
	if frontendBuild(dir) != "/assets/index-newer.js" {
		t.Fatal("live build change not detected")
	}
}

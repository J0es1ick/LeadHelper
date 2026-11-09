package httpapi

import (
	"bytes"
	"encoding/base64"
	"leadhelper/internal/adapters/filepreview"
	"leadhelper/internal/domain"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (a *API) projectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := a.s.ProjectDetail(r.Context(), r.PathValue("id"))
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/projects/sync", func(w http.ResponseWriter, r *http.Request) {
		v, e := a.s.ProjectSyncStatus(r.Context())
		respond(w, v, e)
	})
	mux.HandleFunc("POST /api/projects/sync", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"ok": true}, a.s.SyncProjects(r.Context()))
	})
	mux.HandleFunc("POST /api/projects/{id}/resolve", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Version   int  `json:"version"`
			UseRemote bool `json:"useRemote"`
		}
		if !decode(w, r, &in) {
			return
		}
		respond(w, map[string]bool{"ok": true}, a.s.ResolveProjectConflict(r.Context(), r.PathValue("id"), in.Version, in.UseRemote))
	})
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		v, e := a.s.Projects(r.Context(), domain.ProjectQuery{IncludeHidden: q.Get("includeHidden") == "true", Search: q.Get("search"), State: q.Get("state"), Group: q.Get("group"), Folder: q.Get("folder"), Unfiled: q.Get("unfiled") == "true", Page: page, Limit: limit})
		respond(w, v, e)
	})
	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var p domain.Project
		if !decode(w, r, &p) {
			return
		}
		respond(w, map[string]bool{"ok": true}, a.s.SaveProject(r.Context(), p))
	})
	mux.HandleFunc("POST /api/projects/import/parse", func(w http.ResponseWriter, r *http.Request) {
		if e := a.s.CheckProjects(r.Context()); e != nil {
			respond(w, nil, e)
			return
		}
		var in struct {
			Name string `json:"name"`
			Data string `json:"data"`
		}
		if !decode(w, r, &in) {
			return
		}
		ext := strings.ToLower(filepath.Ext(in.Name))
		b, e := base64.StdEncoding.DecodeString(in.Data)
		if e != nil || len(b) > 5<<20 || len(in.Name) > 300 || (ext != ".xlsx" && ext != ".csv" && ext != ".tsv") {
			send(w, 400, map[string]string{"error": "Выберите XLSX, CSV или TSV до 5 МБ"})
			return
		}
		if ext != ".xlsx" && !utf8.Valid(b) && !bytes.HasPrefix(b, []byte{0xff, 0xfe}) && !bytes.HasPrefix(b, []byte{0xfe, 0xff}) {
			send(w, 400, map[string]string{"error": "Сохраните CSV/TSV в кодировке UTF-8 и загрузите снова"})
			return
		}
		p := filepreview.ImportSpreadsheet(in.Name, b)
		if p.Kind != "table" || p.Truncated {
			send(w, 400, map[string]string{"error": "Таблица повреждена или превышает лимит: 5000 строк на лист, 400 столбцов, 40 листов, 300 000 ячеек и 2 МБ текста. Разделите её на части."})
			return
		}
		send(w, 200, p)
	})
	mux.HandleFunc("POST /api/projects/import/preview-batch", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Batches []domain.ProjectImportBatch `json:"batches"`
			Merge   bool                        `json:"merge"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := a.s.PreviewProjectBatches(r.Context(), in.Batches, in.Merge)
		respond(w, v, e)
	})
	mux.HandleFunc("POST /api/projects/import/preview", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Projects []domain.Project `json:"projects"`
			Merge    bool             `json:"merge"`
			Fields   []string         `json:"fields"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := a.s.PreviewProjects(r.Context(), in.Projects, in.Merge, in.Fields)
		respond(w, v, e)
	})
	mux.HandleFunc("POST /api/projects/import", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Projects  []domain.Project `json:"projects"`
			RequestID string           `json:"requestId"`
		}
		if !decode(w, r, &in) {
			return
		}
		respond(w, map[string]bool{"ok": true}, a.s.ImportProjects(r.Context(), in.Projects, in.RequestID))
	})
}

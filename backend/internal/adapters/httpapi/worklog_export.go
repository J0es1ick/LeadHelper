package httpapi

import (
	"leadhelper/internal/adapters/xlsxexport"
	"leadhelper/internal/domain"
	"net/http"
	"strconv"
)

func (a *API) worklogExportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/jira/worklog-exports", func(w http.ResponseWriter, r *http.Request) {
		var in domain.WorklogExportRequest
		if !decode(w, r, &in) {
			return
		}
		value, e := a.s.StartWorklogExport(r.Context(), in)
		respond(w, value, e)
	})
	mux.HandleFunc("GET /api/jira/worklog-exports/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, _, e := a.s.WorklogExport(r.PathValue("id"))
		respond(w, value, e)
	})
	mux.HandleFunc("DELETE /api/jira/worklog-exports/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"ok": true}, a.s.CancelWorklogExport(r.PathValue("id")))
	})
	mux.HandleFunc("GET /api/jira/worklog-exports/{id}/file", func(w http.ResponseWriter, r *http.Request) {
		status, report, e := a.s.WorklogExport(r.PathValue("id"))
		if e != nil {
			respond(w, nil, e)
			return
		}
		if status.State != "ready" {
			send(w, 409, map[string]string{"error": "Файл ещё не готов. Дождитесь завершения выгрузки"})
			return
		}
		data, e := xlsxexport.Worklogs(report)
		if e != nil {
			respond(w, nil, e)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="leadhelper-hours-`+report.From+`-`+report.To+`.xlsx"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})
}

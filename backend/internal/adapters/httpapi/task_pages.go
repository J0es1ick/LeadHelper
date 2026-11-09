package httpapi

import (
	"leadhelper/internal/application"
	"leadhelper/internal/domain"
	"net/http"
	"strconv"
)

func taskQuery(r *http.Request) (domain.TaskQuery, error) {
	v := r.URL.Query()
	q := domain.TaskQuery{Page: 1, Limit: 25, Personal: "false", Assignee: "all", Status: v.Get("status"), Source: v.Get("source"), Priority: v.Get("priority"), Search: v.Get("search"), Sort: v.Get("sort"), From: v.Get("from"), To: v.Get("to")}
	if v.Has("assignee") {
		q.Assignee = v.Get("assignee")
	}
	if v.Has("personal") {
		q.Personal = v.Get("personal")
	}
	for key, target := range map[string]*int{"page": &q.Page, "limit": &q.Limit} {
		if v.Has(key) {
			n, e := strconv.Atoi(v.Get(key))
			if e != nil || n < 1 || n > 1000000 {
				return q, application.ValidationError{Message: "Некорректная страница задач"}
			}
			*target = n
		}
	}
	return q, nil
}
func (a *API) taskRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		q, e := taskQuery(r)
		if e != nil {
			respond(w, nil, e)
			return
		}
		v, e := a.s.TaskPage(r.Context(), q)
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := a.s.Task(r.Context(), r.PathValue("id"))
		respond(w, v, e)
	})
	mux.HandleFunc("PATCH /api/tasks/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			Status string `json:"status"`
		}
		if !decode(w, r, &v) {
			return
		}
		respond(w, map[string]bool{"ok": true}, a.s.TaskStatus(r.Context(), r.PathValue("id"), v.Status))
	})
	mux.HandleFunc("GET /api/team/plan", func(w http.ResponseWriter, r *http.Request) {
		v, e := a.s.TeamWeek(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/team/day/{member}/{date}", func(w http.ResponseWriter, r *http.Request) {
		q, e := taskQuery(r)
		if e != nil {
			respond(w, nil, e)
			return
		}
		v, e := a.s.TeamDay(r.Context(), r.PathValue("member"), r.PathValue("date"), q)
		respond(w, v, e)
	})
	mux.HandleFunc("PUT /api/team/day/{member}/{date}", func(w http.ResponseWriter, r *http.Request) {
		var v domain.TeamDayInput
		if !decode(w, r, &v) {
			return
		}
		respond(w, map[string]bool{"ok": true}, a.s.SaveTeamDay(r.Context(), r.PathValue("member"), r.PathValue("date"), v))
	})
}

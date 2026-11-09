package httpapi

import (
	"context"
	"leadhelper/internal/domain"
	"net/http"
	"strconv"
	"time"
)

func (a *API) jiraFeatureRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/jira/create/projects", func(w http.ResponseWriter, r *http.Request) {
		v, e := a.s.JiraCreateProjects(r.Context())
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/jira/create/types", func(w http.ResponseWriter, r *http.Request) {
		v, e := a.s.JiraCreateTypes(r.Context(), r.URL.Query().Get("project"))
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/jira/create/fields", func(w http.ResponseWriter, r *http.Request) {
		v, e := a.s.JiraCreateFields(r.Context(), r.URL.Query().Get("project"), r.URL.Query().Get("type"))
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/jira/create/assignees", func(w http.ResponseWriter, r *http.Request) {
		v, e := a.s.JiraAssignable(r.Context(), r.URL.Query().Get("project"), r.URL.Query().Get("search"))
		respond(w, v, e)
	})
	mux.HandleFunc("POST /api/jira/issues", func(w http.ResponseWriter, r *http.Request) {
		var in domain.JiraCreateInput
		if !decode(w, r, &in) {
			return
		}
		v, e := a.s.CreateJiraIssue(r.Context(), in)
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/jira/worklogs", func(w http.ResponseWriter, r *http.Request) {
		v, e := a.s.JiraWorklogWeek(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		respond(w, v, e)
	})
	mux.HandleFunc("POST /api/jira/worklogs/sync", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			From  string `json:"from"`
			To    string `json:"to"`
			Force bool   `json:"force"`
		}
		if !decode(w, r, &in) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		v, e := a.s.SyncJiraWorklogs(ctx, in.From, in.To, in.Force)
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/jira/worklogs/day", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit == 0 {
			limit = 25
		}
		v, e := a.s.JiraWorklogDay(r.Context(), q.Get("member"), q.Get("date"), page, limit)
		respond(w, v, e)
	})
}

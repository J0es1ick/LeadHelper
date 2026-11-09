package httpapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"leadhelper/internal/adapters/mailformat"
	"leadhelper/internal/application"
	"leadhelper/internal/domain"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type API struct {
	s           *application.Service
	static      string
	remoteFiles *remoteFileCache
}

func New(s *application.Service, static string) http.Handler {
	a := &API{s: s, static: static, remoteFiles: &remoteFileCache{files: map[string]remoteFile{}, slots: make(chan struct{}, 2)}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]string{"status": "ok", "version": "0.2.14", "frontendBuild": frontendBuild(static)})
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) { v, e := s.WorkspaceState(r.Context()); respond(w, v, e) })
	a.taskRoutes(mux)
	a.jiraFeatureRoutes(mux)
	a.worklogExportRoutes(mux)
	mux.HandleFunc("POST /api/bitrix/chats/{id}/files", a.bitrixUpload)
	mux.HandleFunc("GET /api/bitrix/files/{id}", a.bitrixFile)
	mux.HandleFunc("GET /api/bitrix/files/{id}/preview", a.bitrixFilePreview)
	mux.HandleFunc("GET /api/bitrix/files/{id}/preview/content", a.bitrixFileContent)
	mux.HandleFunc("GET /api/connections", func(w http.ResponseWriter, r *http.Request) { send(w, 200, s.Connections()) })
	mux.HandleFunc("GET /api/connections/{provider}", func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, s.ConnectionConfig(r.PathValue("provider")))
	})
	mux.HandleFunc("PUT /api/connections/{provider}", func(w http.ResponseWriter, r *http.Request) {
		var c domain.IntegrationConfig
		if !decode(w, r, &c) {
			return
		}
		respond(w, map[string]bool{"ok": true}, s.Configure(r.Context(), r.PathValue("provider"), c))
	})
	mux.HandleFunc("GET /api/workspace/{provider}", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		v, e := s.Browse(r.Context(), r.PathValue("provider"), domain.RemoteQuery{Kind: q.Get("kind"), ID: q.Get("id"), Search: q.Get("search"), Cursor: q.Get("cursor"), Status: q.Get("status"), Department: q.Get("department")})
		respond(w, v, e)
	})
	a.projectRoutes(mux)
	mux.HandleFunc("POST /api/team/bitrix/sync", func(w http.ResponseWriter, r *http.Request) { v, e := s.SyncTeam(r.Context(), false); respond(w, v, e) })
	mux.HandleFunc("GET /api/jira/team", func(w http.ResponseWriter, r *http.Request) { v, e := s.JiraTeamStatus(r.Context()); respond(w, v, e) })
	mux.HandleFunc("GET /api/jira/users", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.JiraUsers(r.Context(), r.URL.Query().Get("search"))
		respond(w, v, e)
	})
	mux.HandleFunc("POST /api/jira/team/match", func(w http.ResponseWriter, r *http.Request) { v, e := s.MatchJiraTeam(r.Context()); respond(w, v, e) })
	mux.HandleFunc("PUT /api/jira/team/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UserID string `json:"userId"`
		}
		if !decode(w, r, &body) {
			return
		}
		v, e := s.LinkJiraMember(r.Context(), r.PathValue("id"), body.UserID)
		respond(w, v, e)
	})
	mux.HandleFunc("PUT /api/team/bitrix", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if !decode(w, r, &body) {
			return
		}
		v, e := s.SetTeamAuto(r.Context(), body.Enabled)
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/calendar", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Calendar(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		respond(w, v, e)
	})
	mux.HandleFunc("POST /api/bitrix/chats/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			Text string `json:"text"`
		}
		if !decode(w, r, &v) {
			return
		}
		respond(w, map[string]bool{"ok": true}, s.SendChat(r.Context(), r.PathValue("id"), v.Text))
	})
	mux.HandleFunc("PATCH /api/notifications/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"ok": true}, s.ReadNotifications(r.Context(), r.PathValue("id")))
	})
	mux.HandleFunc("PATCH /api/bitrix/chats/{id}/read", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MessageID string `json:"messageId"`
			AccountID string `json:"accountId"`
		}
		if !decode(w, r, &body) {
			return
		}
		v, e := s.ReadChat(r.Context(), r.PathValue("id"), body.MessageID, body.AccountID)
		respond(w, v, e)
	})
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var v domain.Settings
		if !decode(w, r, &v) {
			return
		}
		respond(w, map[string]bool{"ok": true}, s.Settings(r.Context(), v))
	})
	mux.HandleFunc("POST /api/tasks", a.task)
	mux.HandleFunc("PUT /api/tasks/{id}", a.task)
	mux.HandleFunc("POST /api/notes", a.note)
	mux.HandleFunc("PUT /api/notes/{id}", a.note)
	mux.HandleFunc("POST /api/members", a.member)
	mux.HandleFunc("PUT /api/members/{id}", a.member)
	mux.HandleFunc("DELETE /api/{kind}/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"ok": true}, s.Delete(r.Context(), r.PathValue("kind"), r.PathValue("id")))
	})
	mux.HandleFunc("POST /api/sync/{provider}", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		v, e := s.Sync(ctx, r.PathValue("provider"))
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/mail-threads", func(w http.ResponseWriter, r *http.Request) {
		q, e := mailQuery(r)
		if e != nil {
			respond(w, nil, e)
			return
		}
		p, e := s.MailPage(r.Context(), q)
		respond(w, p, e)
	})
	mux.HandleFunc("GET /api/mail-threads/{id}", func(w http.ResponseWriter, r *http.Request) {
		q, e := mailQuery(r)
		if e != nil {
			respond(w, nil, e)
			return
		}
		p, e := s.MailThread(r.Context(), r.PathValue("id"), q)
		respond(w, p, e)
	})
	mux.HandleFunc("PATCH /api/mail-threads/{id}", func(w http.ResponseWriter, r *http.Request) {
		var p domain.MailPatch
		if !decode(w, r, &p) {
			return
		}
		e := s.PatchThread(r.Context(), r.PathValue("id"), r.URL.Query().Get("folder"), p)
		respond(w, map[string]bool{"ok": e == nil}, e)
	})
	mux.HandleFunc("GET /api/mail/{id}", func(w http.ResponseWriter, r *http.Request) {
		m, ok := a.findMessage(w, r)
		if ok {
			send(w, 200, m)
		}
	})
	mux.HandleFunc("POST /api/mail/drafts", a.draft)
	mux.HandleFunc("PUT /api/mail/drafts/{id}", a.draft)
	mux.HandleFunc("POST /api/mail/{id}/send", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Send(r.Context(), r.PathValue("id"))
		respond(w, v, e)
	})
	mux.HandleFunc("PATCH /api/mail/{id}", func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			Folder  string `json:"folder"`
			Read    *bool  `json:"read"`
			Starred *bool  `json:"starred"`
		}
		if !decode(w, r, &v) {
			return
		}
		respond(w, map[string]bool{"ok": true}, s.UpdateMessage(r.Context(), r.PathValue("id"), v.Folder, v.Read, v.Starred))
	})
	mux.HandleFunc("GET /api/mail/{id}/export", a.exportMail)
	mux.HandleFunc("GET /api/mail/{id}/attachments/{attachment}", a.attachment)
	mux.HandleFunc("GET /api/mail/{id}/attachments/{attachment}/preview", a.attachmentPreview)
	mux.HandleFunc("GET /api/mail/{id}/attachments/{attachment}/preview/content", a.attachmentContent)
	mux.HandleFunc("POST /api/mail/import", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 7<<20)
		data, e := io.ReadAll(r.Body)
		if e != nil {
			respond(w, nil, application.ValidationError{Message: "EML-файл должен быть до 7 МБ"})
			return
		}
		m, e := mailformat.Decode(data)
		if e != nil {
			respond(w, nil, application.ValidationError{Message: "Не удалось прочитать EML-файл"})
			return
		}
		v, e := s.ImportMessage(r.Context(), m)
		respond(w, v, e)
	})
	mux.HandleFunc("GET /api/reports", a.report)
	mux.HandleFunc("GET /api/reports/export", a.report)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		send(w, 404, map[string]string{"error": "API-маршрут не найден"})
	})
	mux.HandleFunc("/", a.spa)
	return a.protect(mux)
}
func send(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func respond(w http.ResponseWriter, v any, e error) {
	if e == nil {
		send(w, 200, v)
		return
	}
	code := 500
	message := "Внутренняя ошибка. Попробуйте ещё раз"
	var validation application.ValidationError
	if errors.As(e, &validation) {
		code = 400
		message = e.Error()
	} else if errors.Is(e, application.ErrNotFound) {
		code = 404
		message = e.Error()
	} else if errors.Is(e, application.ErrDisabled) {
		code = 403
		message = e.Error()
	} else if errors.Is(e, application.ErrConflict) || errors.Is(e, application.ErrJiraUncertain) {
		code = 409
		message = e.Error()
	} else {
		slog.Error("request failed", "error", e)
	}
	send(w, code, map[string]string{"error": message})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		send(w, 400, map[string]string{"error": "Некорректные данные запроса"})
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		send(w, 400, map[string]string{"error": "Ожидается один JSON-объект"})
		return false
	}
	return true
}
func (a *API) task(w http.ResponseWriter, r *http.Request) {
	var v domain.Task
	if !decode(w, r, &v) {
		return
	}
	out, e := a.s.SaveTask(r.Context(), r.PathValue("id"), v)
	respond(w, out, e)
}
func (a *API) note(w http.ResponseWriter, r *http.Request) {
	var v domain.Note
	if !decode(w, r, &v) {
		return
	}
	out, e := a.s.SaveNote(r.Context(), r.PathValue("id"), v)
	respond(w, out, e)
}
func (a *API) member(w http.ResponseWriter, r *http.Request) {
	var v domain.Member
	if !decode(w, r, &v) {
		return
	}
	out, e := a.s.SaveMember(r.Context(), r.PathValue("id"), v)
	respond(w, out, e)
}
func (a *API) draft(w http.ResponseWriter, r *http.Request) {
	var v domain.Message
	if !decode(w, r, &v) {
		return
	}
	out, e := a.s.SaveDraft(r.Context(), r.PathValue("id"), v)
	respond(w, out, e)
}
func mailQuery(r *http.Request) (domain.MailQuery, error) {
	q := domain.MailQuery{Folder: r.URL.Query().Get("folder"), Search: r.URL.Query().Get("search"), Cursor: r.URL.Query().Get("cursor")}
	if q.Folder == "" {
		q.Folder = "inbox"
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		n, e := strconv.Atoi(value)
		if e != nil {
			return q, application.ValidationError{Message: "Некорректный размер страницы"}
		}
		q.Limit = n
	}
	return q, nil
}
func (a *API) findMessage(w http.ResponseWriter, r *http.Request) (domain.Message, bool) {
	m, e := a.s.Message(r.Context(), r.PathValue("id"))
	if e != nil {
		respond(w, nil, e)
		return m, false
	}
	return m, true
}

func (a *API) exportMail(w http.ResponseWriter, r *http.Request) {
	m, ok := a.findMessage(w, r)
	if !ok {
		return
	}
	b, e := a.s.RawMessage(r.Context(), m)
	if e == nil && b == nil {
		m, e = a.s.MessageWithFiles(r.Context(), m)
		if e == nil {
			b, e = mailformat.Encode(m)
		}
	}
	if e != nil {
		respond(w, nil, e)
		return
	}
	w.Header().Set("Content-Type", "message/rfc822")
	w.Header().Set("Content-Disposition", "attachment; filename=message.eml")
	_, _ = w.Write(b)
}
func (a *API) attachment(w http.ResponseWriter, r *http.Request) {
	m, ok := a.findMessage(w, r)
	if !ok {
		return
	}
	for _, file := range m.Attachments {
		if file.ID == r.PathValue("attachment") {
			data, e := a.s.AttachmentBytes(r.Context(), file)
			if e != nil {
				respond(w, nil, e)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
			_, _ = w.Write(data)
			return
		}
	}
	respond(w, nil, application.ErrNotFound)
}
func safeCell(v string) string {
	if strings.ContainsAny(strings.TrimLeft(v, " \t\r\n")[:min(1, len(strings.TrimLeft(v, " \t\r\n")))], "=+-@") {
		return "'" + v
	}
	return v
}
func (a *API) report(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v, e := a.s.Report(r.Context(), q.Get("from"), q.Get("to"), q.Get("source"))
	if e != nil {
		respond(w, nil, e)
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/export") {
		send(w, 200, v)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=leadhelper-report.csv")
	_, _ = w.Write([]byte{0xef, 0xbb, 0xbf})
	csvw := csv.NewWriter(w)
	csvw.Comma = ';'
	_ = csvw.Write([]string{"ID", "Задача", "Проект", "Источник", "Статус", "Приоритет", "Исполнитель ID", "Оценка, ч", "Срок", "План"})
	for _, t := range v.Tasks {
		row := []string{t.ID, t.Title, t.Project, t.Source, t.Status, t.Priority, t.AssigneeID, strconv.FormatFloat(t.Estimate, 'f', 1, 64), t.DueDate, t.PlannedDate}
		for i := range row {
			row[i] = safeCell(row[i])
		}
		_ = csvw.Write(row)
	}
	csvw.Flush()
}

var frontendScript = regexp.MustCompile(`src="(/assets/[^"<>]+\.js)"`)

func frontendBuild(static string) string {
	data, e := os.ReadFile(filepath.Join(static, "index.html"))
	if e != nil {
		return ""
	}
	match := frontendScript.FindSubmatch(data)
	if len(match) < 2 {
		return ""
	}
	return string(match[1])
}
func (a *API) spa(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/office/viewer.html" {
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src data: blob:; font-src data: blob:; connect-src 'none'; frame-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	p := filepath.Join(a.static, filepath.FromSlash(strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), string(filepath.Separator))))
	if info, e := os.Stat(p); e == nil && !info.IsDir() {
		// Windows registry MIME mappings may classify module workers as text/plain.
		switch strings.ToLower(filepath.Ext(p)) {
		case ".html":
			w.Header().Set("Cache-Control", "no-store")
		case ".mjs":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case ".wasm":
			w.Header().Set("Content-Type", "application/wasm")
		}
		http.ServeFile(w, r, p)
		return
	}
	if strings.Contains(filepath.Base(r.URL.Path), ".") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, filepath.Join(a.static, "index.html"))
}
func (a *API) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https: http:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		defer func() {
			if x := recover(); x != nil {
				slog.Error("recovered request panic")
				send(w, 500, map[string]string{"error": "Внутренняя ошибка"})
			}
		}()
		host := r.Host
		if h, e := url.Parse("http://" + host); e == nil {
			host = h.Hostname()
		}
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			send(w, 403, map[string]string{"error": "Этот экземпляр доступен только локально"})
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if r.Header.Get("X-LeadHelper") != "1" {
				send(w, 403, map[string]string{"error": "Отсутствует заголовок X-LeadHelper"})
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				if e != nil || (u.Host != r.Host && u.Host != "localhost:5173" && u.Host != "127.0.0.1:5173") {
					send(w, 403, map[string]string{"error": "Недопустимый источник запроса"})
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

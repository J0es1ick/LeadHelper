package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("Запись не найдена")
var ErrDisabled = errors.New("Модуль отключён в настройках")
var ErrConflict = errors.New("План изменился в другой вкладке. Перезагрузите его перед сохранением")

type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }
func invalid(s string) error            { return ValidationError{s} }

type Service struct {
	chatUploadMu  sync.Mutex
	chatUploads   map[string]*chatUploadReceipt
	exportMu      sync.Mutex
	exportJobs    map[string]*worklogExportJob
	exportClosed  bool
	exportWG      sync.WaitGroup
	projectSource ports.ProjectSheetSource
	projectMu     sync.Mutex
	repo          ports.Repository
	sources       map[string]ports.TaskSource
	mail          ports.MailProvider
	bitrix        ports.WorkspaceSource
	confluence    ports.KnowledgeSource
	config        ports.Configurator
	workspaceMu   sync.Mutex
	teamMu        sync.Mutex
	blobs         ports.BlobStore
	syncMu        sync.Mutex
	worklogMu     sync.Mutex
	mailSyncGate  chan struct{}
	sendMu        sync.Mutex
}

func New(r ports.Repository, s map[string]ports.TaskSource, m ports.MailProvider) *Service {
	return &Service{repo: r, sources: s, mail: m, mailSyncGate: make(chan struct{}, 1)}
}
func (s *Service) WithBlobs(b ports.BlobStore) *Service { s.blobs = b; return s }
func ID(prefix string) string {
	b := make([]byte, 8)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return prefix + hex.EncodeToString(b)
}
func enabled(st *domain.State, module string) error {
	if !st.Settings.Modules[module] {
		return ErrDisabled
	}
	return nil
}
func taskModule(t domain.Task) string {
	if t.Personal {
		return "planner"
	}
	return "backlog"
}
func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, e := time.Parse("2006-01-02", s)
	return e == nil
}
func (s *Service) State(ctx context.Context) (domain.State, error) {
	st, err := s.viewCore(ctx)
	if err != nil {
		return st, err
	}
	return s.visibleState(ctx, st)
}
func (s *Service) visibleState(ctx context.Context, st domain.State) (domain.State, error) {
	demo := st.Settings.Demo
	if r, ok := s.repo.(ports.PagedMailRepository); ok && st.Settings.Modules["mail"] {
		stats, e := r.MailStats(ctx, demo)
		if e != nil {
			return st, e
		}
		st.MailStats = &stats
		page, e := r.MailPage(ctx, demo, domain.MailQuery{Folder: "inbox", Limit: 3})
		if e != nil {
			return st, e
		}
		for _, t := range page.Items {
			st.Messages = append(st.Messages, t.Latest)
		}
	}
	st.Tasks = slices.DeleteFunc(st.Tasks, func(t domain.Task) bool {
		return t.Source == "bitrix" || t.Demo != demo || !st.Settings.Modules[taskModule(t)] || (t.Source != "local" && !st.Settings.Modules[t.Source])
	})
	st.Notes = slices.DeleteFunc(st.Notes, func(n domain.Note) bool { return n.Demo != demo || !st.Settings.Modules["notes"] })
	st.Messages = slices.DeleteFunc(st.Messages, func(m domain.Message) bool { return m.Demo != demo || !st.Settings.Modules["mail"] })
	st.Members = slices.DeleteFunc(st.Members, func(m domain.Member) bool { return m.Demo != demo })
	st.SyncLogs = slices.DeleteFunc(st.SyncLogs, func(l domain.SyncLog) bool { return l.Demo != demo })
	connections := s.Connections()
	st.MailConnection = &connections[len(connections)-1]
	st.MailCheckpoints = nil // Internal resume tokens are not browser state.
	st.NotificationCursors = nil
	if demo {
		st.TeamSync = domain.TeamSyncStatus{}
		st.Notifications = []domain.Notification{}
	}
	if st.Notifications == nil {
		st.Notifications = []domain.Notification{}
	}
	st.Notifications = slices.DeleteFunc(st.Notifications, func(n domain.Notification) bool { return !st.Settings.Modules[n.Provider] })
	if demo || !st.Settings.Modules["mail"] {
		st.MailSync = domain.MailSyncStatus{State: "inactive"}
	}
	// The workspace DTO exposes collections as arrays, including an empty account.
	if st.Tasks == nil {
		st.Tasks = []domain.Task{}
	}
	if st.Members == nil {
		st.Members = []domain.Member{}
	}
	if st.Notes == nil {
		st.Notes = []domain.Note{}
	}
	if st.Messages == nil {
		st.Messages = []domain.Message{}
	}
	if st.SyncLogs == nil {
		st.SyncLogs = []domain.SyncLog{}
	}
	return st, nil
}
func (s *Service) Settings(ctx context.Context, v domain.Settings) error {
	if strings.TrimSpace(v.Name) == "" || strings.TrimSpace(v.Workspace) == "" {
		return invalid("Укажите имя и название пространства")
	}
	if len(v.Name) > 120 || len(v.Workspace) > 120 {
		return invalid("Название слишком длинное")
	}
	if _, e := mail.ParseAddress(v.Email); e != nil {
		return invalid("Некорректный email")
	}
	keys := []string{"planner", "backlog", "projects", "team", "mail", "notes", "reports", "jira", "bitrix", "confluence"}
	for k := range v.Modules {
		if !slices.Contains(keys, k) {
			return invalid("Неизвестный модуль")
		}
	}
	return s.updateCore(ctx, func(st *domain.State) error { st.Settings = v; return nil })
}
func validateTask(t domain.Task) error {
	if strings.TrimSpace(t.Title) == "" || len(t.Title) > 300 {
		return invalid("Название задачи: от 1 до 300 символов")
	}
	if !slices.Contains([]string{"backlog", "todo", "progress", "review", "done"}, t.Status) {
		return invalid("Некорректный статус")
	}
	if !slices.Contains([]string{"low", "medium", "high", "critical"}, t.Priority) {
		return invalid("Некорректный приоритет")
	}
	if t.Estimate < 0 || t.Estimate > 1000 {
		return invalid("Оценка должна быть от 0 до 1000 часов")
	}
	if !validDate(t.DueDate) || !validDate(t.PlannedDate) {
		return invalid("Некорректная дата")
	}
	if len(t.Description) > 30000 || len(t.Project) > 120 || len(t.Tags) > 20 {
		return invalid("Слишком большой объём данных")
	}
	return nil
}
func (s *Service) SaveTask(ctx context.Context, id string, t domain.Task) (domain.Task, error) {
	err := s.updateCore(ctx, func(st *domain.State) error {
		if e := enabled(st, taskModule(t)); e != nil {
			return e
		}
		if e := validateTask(t); e != nil {
			return e
		}
		if t.AssigneeID != "" && !slices.ContainsFunc(st.Members, func(m domain.Member) bool { return m.ID == t.AssigneeID && m.Demo == st.Settings.Demo }) {
			return invalid("Исполнитель не найден")
		}
		if slices.ContainsFunc(st.Members, func(m domain.Member) bool { return m.ID == t.AssigneeID && m.Demo == st.Settings.Demo && m.Inactive }) && !slices.ContainsFunc(st.Tasks, func(old domain.Task) bool {
			return old.ID == id && old.Demo == st.Settings.Demo && old.AssigneeID == t.AssigneeID
		}) {
			return invalid("Сотрудник больше не входит в активную команду")
		}
		t.Title = strings.TrimSpace(t.Title)
		t.UpdatedAt = time.Now().UTC()
		if t.Tags == nil {
			t.Tags = []string{}
		}
		if id == "" {
			t.ID = ID("LH-")
			t.Source = "local"
			t.ExternalID = ""
			t.URL = ""
			t.CreatedAt = t.UpdatedAt
			t.Demo = st.Settings.Demo
			if t.Status == "done" {
				t.CompletedAt = &t.UpdatedAt
			}
			st.Tasks = append(st.Tasks, t)
			return nil
		}
		for i, old := range st.Tasks {
			if old.ID == id && old.Demo == st.Settings.Demo {
				if e := enabled(st, taskModule(old)); e != nil {
					return e
				}
				if old.Source != "local" {
					if e := enabled(st, old.Source); e != nil {
						return e
					}
				}
				t.ID = id
				t.Source = old.Source
				t.ExternalID = old.ExternalID
				t.URL = old.URL
				t.Demo = old.Demo
				t.CreatedAt = old.CreatedAt
				t.CompletedAt = old.CompletedAt
				if t.Status == "done" && old.Status != "done" {
					t.CompletedAt = &t.UpdatedAt
				}
				if t.Status != "done" {
					t.CompletedAt = nil
				}
				st.Tasks[i] = t
				return nil
			}
		}
		return ErrNotFound
	})
	return t, err
}
func (s *Service) Delete(ctx context.Context, kind, id string) error {
	return s.updateCore(ctx, func(st *domain.State) error {
		switch kind {
		case "tasks":
			for i, t := range st.Tasks {
				if t.ID == id && t.Demo == st.Settings.Demo {
					if e := enabled(st, taskModule(t)); e != nil {
						return e
					}
					if t.Source != "local" {
						return invalid("Импортированную задачу можно изменить в источнике")
					}
					st.Tasks = append(st.Tasks[:i], st.Tasks[i+1:]...)
					st.TeamPlan = slices.DeleteFunc(st.TeamPlan, func(p domain.TeamPlanEntry) bool { return p.TaskID == id && p.Demo == t.Demo })
					return nil
				}
			}
		case "notes":
			if e := enabled(st, "notes"); e != nil {
				return e
			}
			for i, n := range st.Notes {
				if n.ID == id && n.Demo == st.Settings.Demo {
					st.Notes = append(st.Notes[:i], st.Notes[i+1:]...)
					return nil
				}
			}
		}
		return ErrNotFound
	})
}
func (s *Service) SaveNote(ctx context.Context, id string, n domain.Note) (domain.Note, error) {
	err := s.updateCore(ctx, func(st *domain.State) error {
		if e := enabled(st, "notes"); e != nil {
			return e
		}
		n.Title = strings.TrimSpace(n.Title)
		if n.Title == "" || len(n.Title) > 200 || len(n.Body) > 100000 {
			return invalid("Укажите заголовок до 200 символов; текст — до 100 000")
		}
		n.UpdatedAt = time.Now().UTC()
		n.Demo = st.Settings.Demo
		if id == "" {
			n.ID = ID("note-")
			st.Notes = append(st.Notes, n)
			return nil
		}
		for i, old := range st.Notes {
			if old.ID == id && old.Demo == st.Settings.Demo {
				n.ID = id
				st.Notes[i] = n
				return nil
			}
		}
		return ErrNotFound
	})
	return n, err
}
func (s *Service) SaveMember(ctx context.Context, id string, m domain.Member) (domain.Member, error) {
	err := s.updateCore(ctx, func(st *domain.State) error {
		if e := enabled(st, "team"); e != nil {
			return e
		}
		if strings.TrimSpace(m.Name) == "" || len(m.Name) > 120 || m.Capacity < 0 || m.Capacity > 80 {
			return invalid("Укажите имя и доступность от 0 до 80 ч/нед.")
		}
		m.Demo = st.Settings.Demo
		if id == "" {
			m.Email, m.JiraUserID, m.JiraLogin, m.JiraName, m.JiraSite = "", "", "", "", ""
			m.Source = ""
			m.ExternalID = ""
			m.AccountID = ""
			m.Inactive = false
			m.Departments = nil
			m.ID = ID("member-")
			m.Color = "#497968"
			st.Members = append(st.Members, m)
			return nil
		}
		for i, old := range st.Members {
			if old.ID == id && old.Demo == st.Settings.Demo {
				m.ID = id
				m.Color = old.Color
				m.Source = old.Source
				m.ExternalID = old.ExternalID
				m.AccountID = old.AccountID
				m.Inactive = old.Inactive
				m.Departments = old.Departments
				m.Email, m.JiraUserID, m.JiraLogin, m.JiraName, m.JiraSite = old.Email, old.JiraUserID, old.JiraLogin, old.JiraName, old.JiraSite
				if old.Source == "bitrix" {
					m.Name = old.Name
					m.Role = old.Role
				}
				st.Members[i] = m
				return nil
			}
		}
		return ErrNotFound
	})
	return m, err
}
func (s *Service) Connections() []domain.Connection {
	out := []domain.Connection{}
	for _, p := range []struct{ id, name, hint string }{{"jira", "Jira", "JIRA_URL, JIRA_API_TOKEN"}} {
		source := s.sources[p.id]
		out = append(out, domain.Connection{ID: p.id, Name: p.name, Configured: source != nil && source.Configured(), Hint: p.hint})
	}
	out = append(out, domain.Connection{ID: "bitrix", Name: "Битрикс24", Configured: s.bitrix != nil && s.bitrix.Configured(), Hint: "Входящий вебхук: im, log, user, department, calendar, disk"}, domain.Connection{ID: "confluence", Name: "Confluence", Configured: s.confluence != nil && s.confluence.Configured(), Hint: "Адрес сервера и персональный токен"})
	name, hint := "Почта · IMAP", "IMAP_HOST, IMAP_PORT, IMAP_USER, IMAP_PASSWORD_FILE"
	if s.mail != nil {
		if _, ok := s.mail.(ports.IncrementalMailProvider); !ok {
			name, hint = "Microsoft 365", "MS_GRAPH_ACCESS_TOKEN"
		}
	}
	email, host := "", ""
	if identity, ok := s.mail.(interface{ MailIdentity() (string, string) }); ok && s.mail.Configured() {
		email, host = identity.MailIdentity()
	}
	sendConfigured := false
	if sender, ok := s.mail.(interface{ SendConfigured() bool }); ok {
		sendConfigured = sender.SendConfigured()
	}
	if sendConfigured && name == "Почта · IMAP" {
		name = "Почта · IMAP + SMTP"
	}
	return append(out, domain.Connection{ID: "mail", Name: name, Configured: s.mail != nil && s.mail.Configured(), Hint: hint, Email: email, Host: host, SendConfigured: sendConfigured})
}
func (s *Service) Sync(ctx context.Context, provider string) (domain.SyncLog, error) {
	if provider == "bitrix" || provider == "confluence" {
		return s.syncWorkspace(ctx, provider)
	}
	if provider == "mail" {
		if p, ok := s.mail.(ports.IncrementalMailProvider); ok {
			return s.syncMail(ctx, p)
		}
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	st, e := s.viewCore(ctx)
	if e != nil {
		return domain.SyncLog{}, e
	}
	if e = enabled(&st, provider); e != nil {
		return domain.SyncLog{}, e
	}
	if provider != "mail" && provider != "jira" && provider != "bitrix" {
		return domain.SyncLog{}, invalid("Неизвестная интеграция")
	}
	l := domain.SyncLog{ID: ID("sync-"), Provider: provider, At: time.Now().UTC(), Demo: st.Settings.Demo, Success: true}
	var tasks []domain.Task
	var teamRoster []domain.Member
	var teamFingerprint string
	var members []domain.Member
	var messages []domain.Message
	if st.Settings.Demo {
		l.Message = "Демонстрационная синхронизация. Внешние сервисы не вызывались."
	} else if provider == "mail" {
		if s.mail == nil || !s.mail.Configured() {
			e = invalid("Укажите MS_GRAPH_ACCESS_TOKEN в .env и перезапустите сервер")
		} else {
			messages, e = s.mail.FetchMessages(ctx)
		}
	} else {
		p := s.sources[provider]
		if p == nil || !p.Configured() {
			e = invalid("Интеграция не настроена. Добавьте параметры в .env и перезапустите сервер")
		} else if scoped, ok := p.(ports.TeamTaskSource); ok && scoped.TeamScoped() {
			teamRoster, e = s.resolveJiraTeam(ctx, scoped)
			if e == nil {
				teamFingerprint = jiraMembersFingerprint(teamRoster)
				tasks, e = scoped.FetchTeamTasks(ctx, teamRoster)
			}
		} else {
			tasks, members, e = p.FetchTasks(ctx)
		}
	}
	if e != nil {
		l.Success = false
		l.Message = e.Error()
	} else {
		l.Count = len(tasks) + len(messages)
		if !st.Settings.Demo {
			l.Message = fmt.Sprintf("Обновлено записей: %d", l.Count)
		}
	}
	err := s.updateMail(ctx, domain.MailScope{Batch: messages}, func(current *domain.State) error {
		if current.Settings.Demo != st.Settings.Demo {
			return invalid("Режим изменился во время синхронизации")
		}
		if x := enabled(current, provider); x != nil {
			return x
		}
		if e == nil {
			if teamRoster != nil {
				if jiraTeamFingerprint(*current) != teamFingerprint {
					return invalid("Состав команды изменился во время загрузки Jira; повторите синхронизацию")
				}
				mergeLegacyJiraMembers(current, teamRoster)
			}
			if provider == "jira" && !st.Settings.Demo {
				visible := map[string]bool{}
				for _, t := range tasks {
					visible[t.ExternalID] = true
				}
				current.Tasks = slices.DeleteFunc(current.Tasks, func(t domain.Task) bool { return !t.Demo && t.Source == "jira" && !visible[t.ExternalID] })
			}
			for _, m := range members {
				index := slices.IndexFunc(current.Members, func(v domain.Member) bool { return v.ID == m.ID && !v.Demo })
				if index < 0 {
					current.Members = append(current.Members, m)
				} else {
					m.Capacity = current.Members[index].Capacity
					m.Color = current.Members[index].Color
					current.Members[index] = m
				}
			}
			for _, t := range tasks {
				found := false
				for i, old := range current.Tasks {
					if old.Source == provider && old.ExternalID == t.ExternalID && !old.Demo {
						t.ID = old.ID
						t.PlannedDate = old.PlannedDate
						t.Personal = old.Personal
						current.Tasks[i] = t
						found = true
						break
					}
				}
				if !found {
					current.Tasks = append(current.Tasks, t)
				}
			}
			for _, m := range messages {
				found := false
				for _, old := range current.Messages {
					if old.ExternalID == m.ExternalID && !old.Demo {
						found = true
						break
					}
				}
				if !found {
					current.Messages = append(current.Messages, m)
				}
			}
		}
		current.SyncLogs = append([]domain.SyncLog{l}, current.SyncLogs...)
		if len(current.SyncLogs) > 100 {
			current.SyncLogs = current.SyncLogs[:100]
		}
		return nil
	})
	return l, err
}

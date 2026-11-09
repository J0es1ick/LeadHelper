package application

import (
	"context"
	"fmt"
	"leadhelper/internal/bitrixtext"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"strconv"
	"strings"
	"time"
)

func (s *Service) WithWorkspace(b ports.WorkspaceSource, c ports.KnowledgeSource, config ports.Configurator) *Service {
	s.bitrix = b
	s.confluence = c
	s.config = config
	return s
}
func (s *Service) UpgradeIntegrations(ctx context.Context) error {
	return s.updateCore(ctx, func(st *domain.State) error {
		if st.Settings.Modules == nil {
			st.Settings.Modules = map[string]bool{}
		}
		if _, exists := st.Settings.Modules["confluence"]; !exists {
			st.Settings.Modules["confluence"] = true
		}
		if _, exists := st.Settings.Modules["projects"]; !exists {
			st.Settings.Modules["projects"] = true
		}
		return nil
	})
}
func (s *Service) Configure(ctx context.Context, id string, c domain.IntegrationConfig) error {
	if id == "bitrix" {
		s.workspaceMu.Lock()
		defer s.workspaceMu.Unlock()
	}
	if id == "jira" {
		s.syncMu.Lock()
		defer s.syncMu.Unlock()
	}
	if s.config == nil {
		return invalid("Настройки подключений недоступны")
	}
	if e := s.config.Configure(ctx, id, c); e != nil {
		return invalid(e.Error())
	}
	return nil
}
func (s *Service) ConnectionConfig(id string) domain.IntegrationConfig {
	if s.config == nil {
		return domain.IntegrationConfig{}
	}
	return s.config.PublicConfig(id)
}
func (s *Service) workspaceEnabled(ctx context.Context, provider string) error {
	st, e := s.taskShell(ctx)
	if e != nil {
		return e
	}
	if e = enabled(&st, provider); e != nil {
		return e
	}
	if st.Settings.Demo {
		return invalid("Внешние данные доступны в рабочем режиме. Отключите деморежим в настройках")
	}
	return nil
}
func (s *Service) Browse(ctx context.Context, provider string, q domain.RemoteQuery) (domain.RemotePage, error) {
	if e := s.workspaceEnabled(ctx, provider); e != nil {
		return domain.RemotePage{}, e
	}
	if len(q.Search) > 200 || len(q.ID) > 150 || len(q.Cursor) > 250 || len(q.Status) > 30 || len(q.Department) > 30 {
		return domain.RemotePage{}, invalid("Слишком длинный запрос")
	}
	var out domain.RemotePage
	var e error
	if provider == "bitrix" && s.bitrix != nil {
		out, e = s.bitrix.Browse(ctx, q)
	} else if provider == "confluence" && s.confluence != nil {
		out, e = s.confluence.Browse(ctx, q)
	} else {
		return out, invalid("Интеграция не подключена")
	}
	if e != nil {
		return out, invalid(e.Error())
	}
	if provider == "bitrix" && q.Kind == "chats" {
		if e = s.reconcileChatNotifications(ctx, out); e != nil {
			return out, e
		}
	}
	return out, nil
}
func (s *Service) Calendar(ctx context.Context, from, to string) (domain.CalendarPage, error) {
	if e := s.workspaceEnabled(ctx, "bitrix"); e != nil {
		return domain.CalendarPage{}, e
	}
	a, e := time.Parse("2006-01-02", from)
	if e != nil {
		return domain.CalendarPage{}, invalid("Некорректная дата начала")
	}
	z, e := time.Parse("2006-01-02", to)
	if e != nil || z.Before(a) || z.Sub(a) > 31*24*time.Hour {
		return domain.CalendarPage{}, invalid("Выберите период не более 31 дня")
	}
	if s.bitrix == nil || !s.bitrix.Configured() {
		return domain.CalendarPage{}, invalid("Подключите Битрикс24 в разделе «Интеграции», чтобы видеть занятость")
	}
	out, e := s.bitrix.Calendar(ctx, from, to)
	if e != nil {
		return out, invalid(e.Error())
	}
	return out, nil
}
func (s *Service) SendChat(ctx context.Context, id, body string) error {
	if e := s.workspaceEnabled(ctx, "bitrix"); e != nil {
		return e
	}
	if s.bitrix == nil {
		return invalid("Битрикс24 не подключён")
	}
	if e := s.bitrix.SendChat(ctx, id, body); e != nil {
		return invalid(e.Error() + ". Если соединение прервалось, проверьте чат перед повторной отправкой")
	}
	return nil
}
func addNotification(st *domain.State, n domain.Notification) {
	for _, old := range st.Notifications {
		if old.ID == n.ID {
			return
		}
	}
	st.Notifications = append([]domain.Notification{n}, st.Notifications...)
	if len(st.Notifications) > 200 {
		st.Notifications = st.Notifications[:200]
	}
}
func (s *Service) ReadNotifications(ctx context.Context, id string) error {
	return s.updateCore(ctx, func(st *domain.State) error {
		for i := range st.Notifications {
			if id == "all" || st.Notifications[i].ID == id {
				st.Notifications[i].Read = true
			}
		}
		return nil
	})
}
func (s *Service) syncWorkspace(ctx context.Context, provider string) (domain.SyncLog, error) {
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	l := domain.SyncLog{ID: ID("sync-"), Provider: provider, At: time.Now().UTC()}
	if e := s.workspaceEnabled(ctx, provider); e != nil {
		return l, e
	}
	st, e := s.viewCore(ctx)
	if e != nil {
		return l, e
	}
	notifications := []domain.Notification{}
	readChats := []domain.RemoteItem{}
	key, newCursor := "", ""
	if provider == "confluence" {
		if s.confluence == nil {
			e = invalid("Confluence не подключён")
		} else {
			var page domain.RemotePage
			page, e = s.confluence.Browse(ctx, domain.RemoteQuery{Kind: "spaces"})
			l.Count = len(page.Items)
		}
	} else {
		if s.bitrix == nil || !s.bitrix.Configured() {
			return l, invalid("Битрикс24 не подключён")
		}
		key = s.bitrix.AccountID() + ":messages"
		previous := st.NotificationCursors[key]
		maxID, _ := strconv.ParseInt(previous, 10, 64)
		newMax := maxID
		cursor := ""
		for pageNo := 0; pageNo < 40; pageNo++ {
			var page domain.RemotePage
			page, e = s.bitrix.Browse(ctx, domain.RemoteQuery{Kind: "chats", Cursor: cursor})
			if e != nil {
				break
			}
			fresh := false
			for _, item := range page.Items {
				readChats = append(readChats, item)
				id, _ := strconv.ParseInt(item.MessageID, 10, 64)
				if id > newMax {
					newMax = id
				}
				if id > maxID {
					fresh = true
				}
				if previous != "" && id > maxID && item.Unread > 0 && !item.Own {
					notifications = append(notifications, domain.Notification{ID: key + ":" + item.MessageID, Provider: "bitrix", Title: bitrixtext.Plain(item.Title), Text: clipText(bitrixtext.Plain(item.Text), 200), Target: item.ID, At: time.Now().UTC()})
				}
			}
			l.Count += len(page.Items)
			if page.Next == "" || previous != "" && !fresh {
				break
			}
			if page.Next == cursor {
				e = invalid("Битрикс24 вернул повторную страницу чатов")
				break
			}
			cursor = page.Next
			if pageNo == 39 {
				e = invalid("Не удалось дочитать новые чаты: превышен лимит страниц; повторная проверка сохранит прогресс")
			}
		}
		newCursor = strconv.FormatInt(newMax, 10)
	}
	l.Success = e == nil
	l.Message = fmt.Sprintf("Подключение проверено, получено записей: %d", l.Count)
	if e != nil {
		l.Message = e.Error()
	}
	saveErr := s.updateCore(ctx, func(current *domain.State) error {
		if current.Settings.Demo || !current.Settings.Modules[provider] {
			return invalid("Настройки изменились во время синхронизации")
		}
		if e == nil && key != "" {
			if current.NotificationCursors == nil {
				current.NotificationCursors = map[string]string{}
			}
			current.NotificationCursors[key] = newCursor
			for _, n := range notifications {
				addNotification(current, n)
			}
			for _, item := range readChats {
				readChatNotifications(current, s.bitrix.AccountID(), item.ID, chatReadThrough(item))
			}
		}
		current.SyncLogs = append([]domain.SyncLog{l}, current.SyncLogs...)
		if len(current.SyncLogs) > 100 {
			current.SyncLogs = current.SyncLogs[:100]
		}
		return nil
	})
	return l, saveErr
}
func clipText(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}
func (s *Service) RunWorkspaceWorker(ctx context.Context) {
	// Independent loops: a slow Jira response cannot delay incoming chat notices.
	runPollJobs(ctx,
		func(ctx context.Context) {
			pollSource(ctx, 10*time.Second, 50*time.Second, func(c context.Context) error {
				st, e := s.viewCore(c)
				if e != nil {
					return e
				}
				if st.Settings.Demo || !st.Settings.Modules["bitrix"] || s.bitrix == nil || !s.bitrix.Configured() {
					return nil
				}
				l, e := s.syncWorkspace(c, "bitrix")
				if e == nil && !l.Success {
					return invalid(l.Message)
				}
				return e
			})
		},
		func(ctx context.Context) {
			pollSource(ctx, time.Minute, 90*time.Second, func(c context.Context) error {
				st, e := s.viewCore(c)
				if e != nil {
					return e
				}
				if st.Settings.Demo || !st.Settings.Modules["team"] || !st.Settings.Modules["bitrix"] || !st.TeamSync.Enabled {
					return nil
				}
				_, e = s.SyncTeam(c, false)
				return e
			})
		},
		func(ctx context.Context) {
			pollSource(ctx, time.Minute, 90*time.Second, func(c context.Context) error {
				st, e := s.viewCore(c)
				if e != nil {
					return e
				}
				source := s.sources["jira"]
				if st.Settings.Demo || !st.Settings.Modules["jira"] || source == nil || !source.Configured() {
					return nil
				}
				l, e := s.Sync(c, "jira")
				if e == nil && !l.Success {
					return invalid(l.Message)
				}
				return e
			})
		},
		s.runWorklogWorker,
	)
}

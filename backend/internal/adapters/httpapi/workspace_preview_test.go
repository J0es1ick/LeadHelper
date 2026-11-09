package httpapi

import (
	"context"
	"fmt"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/application"
	"leadhelper/internal/domain"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Opt-in UI fixture: all provider actions stay in memory; it cannot contact a real service.
// LEADHELPER_UI_FIXTURE=1 go test ./internal/adapters/httpapi -run TestWorkspacePreview -v -timeout 20m
func TestWorkspacePreview(t *testing.T) {
	if os.Getenv("LEADHELPER_UI_FIXTURE") != "1" {
		t.Skip("interactive fixture disabled")
	}
	seed := application.Seed(time.Now())
	seed.Settings.Demo = false
	seed.Settings.Workspace = "Проверка интерфейса"
	seed.Settings.Email = "demo@example.test"
	seed.Settings.Modules["confluence"] = true
	for i := range seed.Tasks {
		seed.Tasks[i].Demo = false
	}
	for i := range seed.Members {
		seed.Members[i].Demo = false
	}
	for i := range seed.Notes {
		seed.Notes[i].Demo = false
	}
	for i := range seed.Messages {
		seed.Messages[i].Demo = false
	}
	// A read outgoing reply must not keep the opened conversation highlighted because older replies or server copies were unread.
	for i := 0; i < 26; i++ {
		seed.Messages = append(seed.Messages, domain.Message{ID: fmt.Sprintf("read-regression-%02d", i), MessageID: fmt.Sprintf("<read-%02d@fixture.test>", i), References: "<read-root@fixture.test>", From: "peer@example.test", FromName: "Тест прочтения", To: seed.Settings.Email, Subject: "Проверка прочитанной переписки", Body: "Предыдущий ответ в переписке.", Folder: "inbox", Date: time.Now().Add(-time.Duration(27-i) * time.Hour)})
	}
	duplicate := seed.Messages[len(seed.Messages)-1]
	duplicate.ID = "read-copy"
	duplicate.Folder = "archive"
	seed.Messages = append(seed.Messages, duplicate)
	seed.Messages = append(seed.Messages, domain.Message{ID: "read-sent", MessageID: "<read-sent@fixture.test>", References: "<read-root@fixture.test>", From: seed.Settings.Email, FromName: "Вы", To: "peer@example.test", Subject: "Проверка прочитанной переписки", Body: "Последнее письмо уже прочитано. После открытия всей переписки зелёный фон и точка должны исчезнуть, включая старые ответы и копии.", Folder: "sent", Read: true, Date: time.Now()})
	seed.Notifications = []domain.Notification{{ID: "fixture-mail", Provider: "mail", Title: "Анна Смирнова", Text: "Материалы к обсуждению проекта", Target: seed.Messages[0].ID, At: time.Now()}, {ID: "fixture-chat", Provider: "bitrix", Title: "Команда продукта", Text: "Согласуем время встречи", Target: "chat42", At: time.Now()}}
	repo, e := storage.OpenSQLite(filepath.Join(t.TempDir(), "preview.sqlite"), "", seed)
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	fixture := &workspacePreview{}
	s := application.New(repo, nil, previewMail{}).WithWorkspace(fixture, previewKnowledge{}, nil)
	server := &http.Server{Addr: "127.0.0.1:8092", Handler: New(s, filepath.Join("..", "..", "..", "..", "frontend", "dist")), ReadHeaderTimeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	go func() { <-ctx.Done(); _ = server.Close() }()
	t.Log("UI fixture: http://localhost:8092 — synthetic data, external sends impossible")
	if e = server.ListenAndServe(); e != http.ErrServerClosed {
		t.Fatal(e)
	}
}

type previewMail struct{}

func (previewMail) Configured() bool                                        { return true }
func (previewMail) SendConfigured() bool                                    { return true }
func (previewMail) MailIdentity() (string, string)                          { return "demo@example.test", "fixture.local" }
func (previewMail) ValidateSend(domain.Message) error                       { return nil }
func (previewMail) FetchMessages(context.Context) ([]domain.Message, error) { return nil, nil }
func (previewMail) Send(context.Context, domain.Message) error              { return nil }

type workspacePreview struct {
	mu       sync.Mutex
	messages []domain.RemoteItem
}

func (*workspacePreview) Configured() bool  { return true }
func (*workspacePreview) AccountID() string { return "ui-fixture" }
func (w *workspacePreview) Browse(_ context.Context, q domain.RemoteQuery) (domain.RemotePage, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := domain.RemotePage{Items: []domain.RemoteItem{}}
	switch q.Kind {
	case "chats":
		p.Items = []domain.RemoteItem{{ID: "chat42", Kind: "chat", Title: "Команда продукта", Text: "Согласуем время встречи", Unread: 2, Date: time.Now()}, {ID: "17", Kind: "chat", Title: "Анна Смирнова", Text: "Отправила материалы по проекту", Date: time.Now().Add(-time.Hour)}}
	case "messages":
		p.Items = append(p.Items, w.messages...)
		p.Items = append(p.Items, domain.RemoteItem{ID: "fixture-2", Kind: "message", Title: "Анна Смирнова", Text: "Согласуем время встречи. В календаре уже забронирована переговорная.", Date: time.Now()}, domain.RemoteItem{ID: "fixture-1", Kind: "message", Title: "Вы", Own: true, Text: "Материалы к обсуждению готовы.", Date: time.Now().Add(-time.Hour)})
	case "news":
		p.Items = []domain.RemoteItem{
			{ID: "news-3", Title: "Новая неделя: планы команды", Author: "Анна Смирнова", Text: "На этой неделе завершаем обновление личного кабинета и начинаем проверку мобильной версии.\n\nОбсудим результаты на общей встрече в понедельник. Материалы доступны в базе знаний.", Date: time.Now(), URL: "https://example.test/news/3"},
			{ID: "news-2", Title: "База знаний стала удобнее", Author: "Максим Волков", Text: "Собрали договорённости по проектам в одном пространстве. Для каждой команды есть отдельный раздел с решениями, инструкциями и полезными ссылками.", Date: time.Now().Add(-24 * time.Hour), URL: "https://example.test/news/2"},
			{ID: "news-1", Title: "Встреча продуктовой команды", Author: "Елена Орлова", Text: "Спасибо за обсуждение! Обновили план работ и распределили задачи на следующий этап.", Date: time.Now().Add(-48 * time.Hour), URL: "https://example.test/news/1"}}
	case "people":
		p.Items = []domain.RemoteItem{{ID: "17", Title: "Анна Смирнова", Text: "Руководитель frontend-разработки", Status: "active", Online: true, Departments: []string{"2"}, URL: "https://example.test/17"}, {ID: "18", Title: "Максим Волков", Text: "Backend developer", Status: "active", Departments: []string{"3"}, URL: "https://example.test/18"}, {ID: "19", Title: "Елена Орлова", Text: "Product designer", Status: "dismissed", Departments: []string{"4"}, URL: "https://example.test/19"}, {ID: "20", Title: "Алексей Петров", Text: "Технический директор", Status: "active", Departments: []string{"1"}, URL: "https://example.test/20"}}
		total := len(p.Items)
		p.TotalAll = &total
		filtered := []domain.RemoteItem{}
		for _, person := range p.Items {
			if q.Status != "" && q.Status != "all" && q.Status != "online" && q.Status != person.Status {
				continue
			}
			if q.Status == "online" && !person.Online {
				continue
			}
			if q.Search != "" && !strings.Contains(strings.ToLower(person.Title), strings.ToLower(q.Search)) {
				continue
			}
			if q.Department != "" && person.Departments[0] != q.Department {
				continue
			}
			filtered = append(filtered, person)
		}
		p.Items = filtered
		count := len(filtered)
		p.Total = &count
	case "departments":
		p.Items = []domain.RemoteItem{{ID: "1", Title: "Компания", HeadID: "20", HeadName: "Алексей Петров", HeadURL: "https://example.test/20"}, {ID: "2", Title: "Продуктовая разработка", Parent: "1", HeadID: "17", HeadName: "Анна Смирнова", HeadURL: "https://example.test/17"}, {ID: "3", Title: "Backend", Parent: "2", HeadID: "18", HeadName: "Максим Волков", HeadURL: "https://example.test/18"}, {ID: "4", Title: "Дизайн", Parent: "1", HeadID: "19", HeadName: "Елена Орлова", HeadURL: "https://example.test/19"}}
		total := len(p.Items)
		p.Total = &total
	case "files":
		if q.ID == "" {
			p.Items = []domain.RemoteItem{{ID: "1", Title: "Материалы команды", Kind: "folder"}}
		} else {
			p.Items = []domain.RemoteItem{{ID: "2", Title: "Требования.pdf", Kind: "file", Text: "Документ проекта"}}
		}
	}
	return p, nil
}
func (w *workspacePreview) SendChat(_ context.Context, _ string, text string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.messages = append([]domain.RemoteItem{{ID: fmt.Sprintf("sent-%d", len(w.messages)), Kind: "message", Own: true, Title: "Я", Text: text, Date: time.Now()}}, w.messages...)
	return nil
}
func (*workspacePreview) Calendar(_ context.Context, from, to string) (domain.CalendarPage, error) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	day := time.Now().In(loc).Format("2006-01-02")
	if day < from || day > to {
		day = from
	}
	at := func(clock string) time.Time { v, _ := time.Parse(time.RFC3339, day+"T"+clock+":00+03:00"); return v }
	return domain.CalendarPage{Timezone: "Europe/Moscow", UpdatedAt: time.Now(), Events: []domain.CalendarEvent{{ID: "meet-1", Title: "Планирование команды", Start: at("10:00"), End: at("11:00"), Busy: true, Location: "Переговорная 2"}, {ID: "meet-2", Title: "Обсуждение архитектуры", Start: at("10:30"), End: at("12:00"), Busy: true, Location: "Онлайн"}, {ID: "meet-3", Title: "Встреча с заказчиком", Start: at("15:00"), End: at("16:00"), Busy: true, Tentative: true, Location: "Онлайн"}}}, nil
}

type previewKnowledge struct{}

func (previewKnowledge) Configured() bool { return true }
func (previewKnowledge) Browse(_ context.Context, q domain.RemoteQuery) (domain.RemotePage, error) {
	p := domain.RemotePage{Items: []domain.RemoteItem{}}
	switch q.Kind {
	case "spaces":
		p.Items = []domain.RemoteItem{{ID: "TEAM", Title: "Продуктовая команда", Kind: "space"}, {ID: "ENG", Title: "Разработка", Kind: "space"}}
	case "pages":
		p.Items = []domain.RemoteItem{{ID: "101", Title: "Договорённости команды", Kind: "page", Text: "Продуктовая команда", Author: "Анна Смирнова", Date: time.Now()}, {ID: "102", Title: "Архитектура проекта", Kind: "page", Text: "Разработка", Date: time.Now()}}
		if q.Search != "" {
			filtered := []domain.RemoteItem{}
			for _, v := range p.Items {
				if strings.Contains(strings.ToLower(v.Title), strings.ToLower(q.Search)) {
					filtered = append(filtered, v)
				}
			}
			p.Items = filtered
		}
	case "page":
		p.Items = []domain.RemoteItem{{ID: q.ID, Title: "Договорённости команды", Kind: "page", HTML: "<h2>Рабочие встречи</h2><p>Планирование проходит по понедельникам. Повестка хранится в Confluence.</p><table><tr><th>Встреча</th><th>Продолжительность</th></tr><tr><td>Планирование</td><td>1 час</td></tr></table><p><strong>Решения</strong> фиксируем на странице проекта.</p>"}}
	}
	return p, nil
}

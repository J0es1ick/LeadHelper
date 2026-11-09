package application

import (
	"encoding/base64"
	"fmt"
	"leadhelper/internal/domain"
	"time"
)

func Seed(now time.Time) domain.State {
	now = now.In(time.FixedZone("MSK", 10800))
	day := func(n int) string { return now.AddDate(0, 0, n).Format("2006-01-02") }
	monday := now.AddDate(0, 0, -(int(now.Weekday())+6)%7)
	planned := func(n int) string { return monday.AddDate(0, 0, n).Format("2006-01-02") }
	st := domain.State{Version: 1, Settings: domain.Settings{Name: "Андрей", Workspace: "Продуктовая команда", Email: "lead@example.test", Demo: true, Modules: map[string]bool{"planner": true, "backlog": true, "team": true, "mail": true, "notes": true, "reports": true, "jira": true, "bitrix": true}}, Tasks: []domain.Task{}, Notes: []domain.Note{}, Messages: []domain.Message{}, SyncLogs: []domain.SyncLog{}}
	st.Members = []domain.Member{{ID: "me", Name: "Андрей Иванов", Role: "Тимлид", Color: "#427a60", Capacity: 24, Demo: true}, {ID: "anna", Name: "Анна Смирнова", Role: "Frontend developer", Color: "#8c78bc", Capacity: 32, Demo: true}, {ID: "max", Name: "Максим Волков", Role: "Backend developer", Color: "#6591b0", Capacity: 32, Demo: true}, {ID: "kate", Name: "Екатерина Белова", Role: "QA engineer", Color: "#c08b57", Capacity: 28, Demo: true}, {ID: "alex", Name: "Алексей Морозов", Role: "Product designer", Color: "#ba7689", Capacity: 24, Demo: true}}
	st.Settings.Modules["projects"] = true
	rows := []struct {
		title, status, priority, source, assignee, project string
		hours                                              float64
		due, plan                                          int
	}{
		{"Обновить авторизацию и refresh-токены", "progress", "high", "jira", "max", "Платформа", 16, 2, 0},
		{"Собрать новый экран аналитики", "progress", "high", "jira", "anna", "Личный кабинет", 20, 3, 1},
		{"Согласовать требования к интеграции CRM", "todo", "critical", "local", "me", "CRM-интеграция", 6, -1, 0},
		{"Проверить регрессию платёжного модуля", "review", "high", "jira", "kate", "Платформа", 12, 1, 2},
		{"Подготовить UI-kit для нового кабинета", "progress", "medium", "local", "alex", "Личный кабинет", 16, 5, 2},
		{"Оптимизировать запросы к каталогу", "todo", "high", "jira", "max", "Платформа", 20, 4, 3},
		{"Добавить состояния загрузки и ошибок", "todo", "medium", "jira", "anna", "Личный кабинет", 8, 4, 3},
		{"Описать контракт обмена с Битрикс24", "backlog", "high", "local", "max", "CRM-интеграция", 10, 8, 7},
		{"Настроить мониторинг очередей", "backlog", "medium", "local", "", "Платформа", 8, 10, 8},
		{"Исследовать сценарии онбординга", "backlog", "low", "local", "alex", "Личный кабинет", 6, 9, 7},
		{"Обновить smoke-тесты", "done", "medium", "jira", "kate", "Платформа", 8, 0, 1},
		{"Подготовить релиз 2.4", "done", "high", "local", "me", "Платформа", 4, 0, 0},
	}
	for i, r := range rows {
		t := domain.Task{ID: fmt.Sprintf("LH-%d", 101+i), Title: r.title, Description: "Демонстрационная задача. Уточните критерии приёмки, оценку и ответственного.\n\nКритерии готовности:\n• Изменения прошли код-ревью\n• Основные сценарии проверены\n• Документация актуализирована", Status: r.status, Priority: r.priority, Source: r.source, AssigneeID: r.assignee, Project: r.project, Estimate: r.hours, DueDate: day(r.due), PlannedDate: planned(r.plan), Tags: []string{"спринт 24"}, CreatedAt: now.AddDate(0, 0, -7), UpdatedAt: now, Demo: true}
		if r.source != "local" {
			t.ExternalID = fmt.Sprintf("TEAM-%d", 241+i)
		}
		if t.Status == "done" {
			v := now
			t.CompletedAt = &v
		}
		st.Tasks = append(st.Tasks, t)
	}
	for i, title := range []string{"Подготовить вопросы к 1:1 с Анной", "Проверить PR по авторизации", "Собрать итоги недели", "Спланировать следующий спринт"} {
		status := "todo"
		if i == 1 {
			status = "done"
		}
		st.Tasks = append(st.Tasks, domain.Task{ID: fmt.Sprintf("MY-%d", i+1), Title: title, Description: "", Status: status, Priority: "medium", Source: "local", AssigneeID: "me", Estimate: 1, DueDate: day(0), PlannedDate: day(0), Personal: true, Project: "Личное", Tags: []string{}, CreatedAt: now, UpdatedAt: now, Demo: true})
	}
	st.Notes = []domain.Note{{ID: "note-1", Title: "Фокус команды · ноябрь", Body: "Главное — закончить авторизацию и снять риски по CRM.\n\n1. Не расширяем объём спринта без обсуждения.\n2. Оставляем время на ревью и поддержку.\n3. В пятницу — короткое демо результатов.\n\nПроверить: есть ли у каждого задачи с понятными критериями готовности?", Tag: "Команда", Pinned: true, UpdatedAt: now, Demo: true}, {ID: "note-2", Title: "1:1 · Анна", Body: "Обсудить:\n• Как идёт работа над аналитикой?\n• Нужна ли помощь с архитектурой компонентов?\n• Что хочется попробовать в следующем квартале?\n\nДоговорённости записать после встречи.", Tag: "1:1", UpdatedAt: now.AddDate(0, 0, -1), Demo: true}, {ID: "note-3", Title: "Идеи для следующей ретро", Body: "Попробовать формат: продолжить / перестать / начать.\n\nОтдельно обсудить скорость ревью и непредвиденные задачи от поддержки.", Tag: "Идеи", UpdatedAt: now.AddDate(0, 0, -2), Demo: true}}
	mailRows := []struct{ name, from, subject, body string }{
		{"Анна Смирнова", "anna@example.test", "Макеты аналитики готовы к обсуждению", "Андрей, привет!\n\nПодготовила первую версию экрана аналитики. Добавила состояния загрузки и пустого списка.\n\nПредлагаю посмотреть вместе после стендапа. Особое внимание — фильтрам по проектам.\n\nАнна"},
		{"Алексей Морозов", "alex@example.test", "UI-kit · обновления компонентов", "Привет!\n\nОбновил таблицы, кнопки и карточки задач. Прикладываю краткий список изменений.\n\nЕсли будут вопросы — напиши.\nАлексей"},
		{"Мария · продукт", "product@example.test", "Приоритеты следующего спринта", "Коллеги, добрый день!\n\nВ следующий спринт предлагаем взять интеграцию CRM и улучшение онбординга. Оцените, пожалуйста, доступность команды.\n\nСпасибо!"},
		{"Битрикс24", "notify@example.test", "CRM: уточнение полей для обмена", "Андрей, добрый день.\n\nДля согласования интеграции нужны обязательные поля: ID клиента, статус сделки, ответственный и дата изменения.\n\nПросьба зафиксировать контракт до начала разработки."},
		{"Максим Волков", "max@example.test", "Ревью: обновление авторизации", "Привет!\n\nPR готов к ревью. Добавлены ротация refresh-токенов и интеграционные тесты.\n\nЕсть вопрос по политике истечения сессий. Обсудим?"},
	}
	for i, r := range mailRows {
		m := domain.Message{ID: fmt.Sprintf("mail-%d", i+1), From: r.from, FromName: r.name, To: st.Settings.Email, Subject: r.subject, Body: r.body, Folder: "inbox", Read: i > 1, Starred: i == 2, Date: now.Add(-time.Duration(i*70) * time.Minute), Attachments: []domain.Attachment{}, Demo: true}
		if i == 1 {
			m.Attachments = append(m.Attachments, domain.Attachment{ID: "a1", Name: "ui-kit-changelog.txt", ContentType: "text/plain", Data: base64.StdEncoding.EncodeToString([]byte("Демонстрационный файл\nОбновлены таблицы, кнопки и карточки задач.\n"))})
		}
		st.Messages = append(st.Messages, m)
	}
	return st
}

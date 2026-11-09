package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"regexp"
	"strings"
	"time"
)

var ErrJiraUncertain = errors.New("Ответ о создании задачи не получен. Проверьте Jira: повторное создание заблокировано, чтобы избежать дубля")
var jiraMetaID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

func (s *Service) issueSource(ctx context.Context) (ports.JiraIssueSource, error) {
	if e := s.workspaceEnabled(ctx, "jira"); e != nil {
		return nil, e
	}
	base := s.sources["jira"]
	p, ok := base.(ports.JiraIssueSource)
	if !ok || !base.Configured() {
		return nil, invalid("Подключите Jira в интеграциях")
	}
	return p, nil
}
func (s *Service) JiraCreateProjects(ctx context.Context) ([]domain.JiraOption, error) {
	p, e := s.issueSource(ctx)
	if e != nil {
		return nil, e
	}
	v, e := p.CreationProjects(ctx)
	if e != nil {
		return nil, invalid(e.Error())
	}
	return v, nil
}
func (s *Service) JiraCreateTypes(ctx context.Context, project string) ([]domain.JiraOption, error) {
	if !jiraMetaID.MatchString(project) {
		return nil, invalid("Выберите проект")
	}
	p, e := s.issueSource(ctx)
	if e != nil {
		return nil, e
	}
	v, e := p.CreationTypes(ctx, project)
	if e != nil {
		return nil, invalid(e.Error())
	}
	return v, nil
}
func (s *Service) JiraCreateFields(ctx context.Context, project, typ string) (domain.JiraCreateMeta, error) {
	if !jiraMetaID.MatchString(project) || !jiraMetaID.MatchString(typ) {
		return domain.JiraCreateMeta{}, invalid("Выберите проект и тип задачи")
	}
	p, e := s.issueSource(ctx)
	if e != nil {
		return domain.JiraCreateMeta{}, e
	}
	v, e := p.CreationFields(ctx, project, typ)
	if e != nil {
		return v, invalid(e.Error())
	}
	return v, nil
}
func (s *Service) JiraAssignable(ctx context.Context, project, search string) ([]domain.JiraUser, error) {
	if !jiraMetaID.MatchString(project) || len(search) > 200 {
		return nil, invalid("Некорректный поиск")
	}
	p, e := s.issueSource(ctx)
	if e != nil {
		return nil, e
	}
	v, e := p.AssignableUsers(ctx, project, search)
	if e != nil {
		return nil, invalid(e.Error())
	}
	return v, nil
}
func (s *Service) CreateJiraIssue(ctx context.Context, in domain.JiraCreateInput) (domain.JiraCreateResult, error) {
	out := domain.JiraCreateResult{}
	if !jiraMetaID.MatchString(in.RequestID) || len(in.RequestID) < 16 || !jiraMetaID.MatchString(in.Project) || !jiraMetaID.MatchString(in.IssueType) {
		return out, invalid("Некорректный запрос создания задачи")
	}
	if len(in.Fields) > 200 || len(in.Attachments) > 10 {
		return out, invalid("Слишком много полей или файлов")
	}
	summary, _ := in.Fields["summary"].(string)
	if strings.TrimSpace(summary) == "" || len([]rune(summary)) > 255 {
		return out, invalid("Название задачи должно содержать от 1 до 255 символов")
	}
	total := 0
	for _, f := range in.Attachments {
		data, e := base64.StdEncoding.DecodeString(f.Data)
		total += len(data)
		if e != nil || f.Name == "" || len(f.Name) > 250 || strings.ContainsAny(f.Name, "/\\\x00\r\n") {
			return out, invalid("Некорректный файл")
		}
	}
	if total > 5<<20 {
		return out, invalid("Суммарный размер вложений — до 5 МБ")
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	p, e := s.issueSource(ctx)
	if e != nil {
		return out, e
	}
	repo, ok := s.repo.(ports.JiraFeatureRepository)
	if !ok {
		return out, invalid("Хранилище операций Jira недоступно")
	}
	site := p.SiteID()
	raw, _ := json.Marshal(in)
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	record, e := repo.JiraWrite(ctx, site, in.RequestID)
	if e != nil {
		return out, e
	}
	if record.Status != "" && record.Status != "rejected" {
		if record.Hash != hash {
			return out, invalid("Этот запрос уже использован. Откройте новую форму")
		}
		if record.Status == "created" {
			return record.Result, nil
		}
		return out, ErrJiraUncertain
	}
	// Persist intent before any external write. Keep finishing the operation after browser disconnects.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer cancel()
	record = domain.JiraWriteRecord{Hash: hash, Status: "pending"}
	if e = repo.PutJiraWrite(ctx, site, in.RequestID, record); e != nil {
		return out, e
	}
	out, e = p.CreateIssue(ctx, in)
	if e != nil {
		var failure *domain.JiraWriteError
		if errors.As(e, &failure) && failure.Definitive {
			record.Status = "rejected"
			_ = repo.PutJiraWrite(ctx, site, in.RequestID, record)
			return out, invalid(e.Error())
		}
		return out, ErrJiraUncertain
	}
	st, e := s.taskShell(ctx)
	if e == nil {
		a, _ := in.Fields["assignee"].(map[string]any)
		remoteAssignee := out.Task.AssigneeID
		out.Task.AssigneeID = ""
		for _, m := range st.Members {
			if m.JiraSite == site && !m.Demo && ((remoteAssignee != "" && (remoteAssignee == m.ID || remoteAssignee == "jira-"+m.JiraUserID || remoteAssignee == "jira-"+m.JiraLogin)) || (remoteAssignee == "" && (m.JiraUserID != "" && m.JiraUserID == a["accountId"] || m.JiraLogin != "" && m.JiraLogin == a["name"] || m.JiraUserID != "" && m.JiraUserID == a["key"]))) {
				out.Task.AssigneeID = m.ID
				break
			}
		}
	}
	record.Status = "created"
	record.Result = out
	if len(in.Attachments) > 0 {
		record.Result.Warnings = append(append([]string{}, out.Warnings...), "Задача создана. Если отправка прервалась, проверьте наличие вложений в Jira")
	}
	// Store the returned key BEFORE optional file uploads. A retry never repeats issue creation.
	if e = repo.PutJiraWrite(ctx, site, in.RequestID, record); e != nil {
		out.Warnings = append(out.Warnings, "Задача создана. Не повторяйте создание: не удалось сохранить журнал операции")
		return out, nil
	}
	out.Warnings = append(out.Warnings, p.UploadIssueFiles(ctx, out.Task.ExternalID, in.Attachments)...)
	e = s.updateCore(ctx, func(st *domain.State) error {
		for _, t := range st.Tasks {
			if t.ID == out.Task.ID && !t.Demo {
				return nil
			}
		}
		st.Tasks = append(st.Tasks, out.Task)
		return nil
	})
	if e != nil {
		out.Warnings = append(out.Warnings, "Задача создана в Jira; обновите бэклог синхронизацией")
	}
	if out.Task.AssigneeID == "" {
		out.Warnings = append(out.Warnings, "Исполнитель не сопоставлен с командой. Задача может не входить в текущий фильтр синхронизации")
	}
	record.Result = out
	_ = repo.PutJiraWrite(ctx, site, in.RequestID, record)
	return out, nil
}
func (s *Service) worklogContext(ctx context.Context) (ports.JiraWorklogSource, ports.JiraFeatureRepository, []domain.Member, string, string, error) {
	if e := s.workspaceEnabled(ctx, "jira"); e != nil {
		return nil, nil, nil, "", "", e
	}
	p, ok := s.sources["jira"].(ports.JiraWorklogSource)
	if !ok || !s.sources["jira"].Configured() {
		return nil, nil, nil, "", "", invalid("Подключите Jira, чтобы видеть списанные часы")
	}
	repo, ok := s.repo.(ports.JiraFeatureRepository)
	if !ok {
		return nil, nil, nil, "", "", invalid("Хранилище списаний недоступно")
	}
	st, e := s.taskShell(ctx)
	if e != nil {
		return nil, nil, nil, "", "", e
	}
	members := []domain.Member{}
	missing := 0
	for _, m := range st.Members {
		if m.Demo || m.Inactive {
			continue
		}
		if m.JiraSite == p.SiteID() && m.JiraUserID != "" && m.JiraLogin != "" {
			members = append(members, m)
		} else {
			missing++
		}
	}
	warning := ""
	if missing > 0 {
		warning = fmt.Sprintf("Без сопоставления с Jira: %d. Их часы пока не загружаются; настройте команду в интеграциях", missing)
	}
	scope := fmt.Sprintf("%s-%x", p.SiteID(), sha256.Sum256([]byte(jiraMembersFingerprint(members))))
	return p, repo, members, scope, warning, nil
}
func worklogDates(from, to string) error {
	a, e := time.Parse("2006-01-02", from)
	if e != nil {
		return invalid("Некорректная дата")
	}
	b, e := time.Parse("2006-01-02", to)
	if e != nil || b.Before(a) || b.Sub(a) > 30*24*time.Hour {
		return invalid("Выберите не более 31 дня")
	}
	return nil
}
func (s *Service) JiraWorklogWeek(ctx context.Context, from, to string) (domain.WorklogWeek, error) {
	out := domain.WorklogWeek{Cells: []domain.WorklogCell{}, Timezone: "Europe/Moscow"}
	if e := worklogDates(from, to); e != nil {
		return out, e
	}
	_, repo, _, scope, warning, e := s.worklogContext(ctx)
	if e != nil {
		return out, e
	}
	out, e = repo.WorklogWeek(ctx, scope, from, to)
	out.Warning = warning
	return out, e
}
func (s *Service) SyncJiraWorklogs(ctx context.Context, from, to string, force bool) (domain.WorklogWeek, error) {
	if e := worklogDates(from, to); e != nil {
		return domain.WorklogWeek{}, e
	}
	s.worklogMu.Lock()
	defer s.worklogMu.Unlock()
	p, repo, members, scope, warning, e := s.worklogContext(ctx)
	if e != nil {
		return domain.WorklogWeek{}, e
	}
	cached, e := repo.WorklogWeek(ctx, scope, from, to)
	if e != nil {
		return cached, e
	}
	cached.Warning = warning
	if !force && cached.SyncedAt != nil && time.Since(*cached.SyncedAt) < time.Minute {
		return cached, nil
	}
	if len(members) == 0 {
		return cached, invalid("Сопоставьте участников команды с Jira в интеграциях")
	}
	rows, e := p.FetchWorklogs(ctx, members, from, to)
	if e != nil {
		return cached, invalid(e.Error() + ". Предыдущие списания сохранены")
	}
	_, _, _, current, _, e := s.worklogContext(ctx)
	if e != nil || current != scope {
		return cached, invalid("Состав команды или подключение изменились. Обновите списания")
	}
	if e = repo.ReplaceWorklogs(ctx, scope, from, to, rows, time.Now()); e != nil {
		return cached, e
	}
	return s.JiraWorklogWeek(ctx, from, to)
}
func (s *Service) JiraWorklogDay(ctx context.Context, member, date string, page, limit int) (domain.WorklogPage, error) {
	if e := worklogDates(date, date); e != nil {
		return domain.WorklogPage{}, e
	}
	_, repo, _, scope, _, e := s.worklogContext(ctx)
	if e != nil {
		return domain.WorklogPage{}, e
	}
	return repo.WorklogDay(ctx, scope, member, date, max(1, page), min(50, max(1, limit)))
}

// Worklog reads must not hold up mail/Bitrix notification polling.
func (s *Service) runWorklogWorker(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		st, e := s.taskShell(ctx)
		if e == nil && !st.Settings.Demo && st.Settings.Modules["team"] && st.Settings.Modules["jira"] {
			_, from, to := moscowWeek()
			c, cancel := context.WithTimeout(ctx, 90*time.Second)
			_, _ = s.SyncJiraWorklogs(c, from, to, false)
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

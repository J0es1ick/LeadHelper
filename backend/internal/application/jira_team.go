package application

import (
	"context"
	"encoding/json"
	"fmt"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"sort"
	"strings"
)

func jiraMembers(st domain.State) []domain.Member {
	out := []domain.Member{}
	if st.TeamSync.LastSuccessAt == nil {
		return out
	}
	for _, m := range st.Members {
		if !m.Demo && !m.Inactive && m.Source == "bitrix" && m.AccountID == st.TeamSync.AccountID {
			out = append(out, m)
		}
	}
	return out
}
func jiraTeamFingerprint(st domain.State) string {
	return jiraMembersFingerprint(jiraMembers(st))
}
func jiraMembersFingerprint(members []domain.Member) string {
	rows := []string{}
	for _, m := range members {
		raw, _ := json.Marshal([]string{m.ID, m.Email, m.JiraSite, m.JiraUserID, m.JiraLogin})
		rows = append(rows, string(raw))
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}
func (s *Service) jiraSource(ctx context.Context) (ports.TeamTaskSource, error) {
	if e := s.workspaceEnabled(ctx, "jira"); e != nil {
		return nil, e
	}
	p, ok := s.sources["jira"].(ports.TeamTaskSource)
	if !ok || !p.Configured() {
		return nil, invalid("Подключите Jira в разделе интеграций")
	}
	return p, nil
}
func (s *Service) JiraTeamStatus(ctx context.Context) (domain.JiraTeamStatus, error) {
	out := domain.JiraTeamStatus{Members: []domain.Member{}, Scope: "team"}
	p, e := s.jiraSource(ctx)
	if e != nil {
		return out, e
	}
	st, e := s.viewCore(ctx)
	if e != nil {
		return out, e
	}
	if !p.TeamScoped() {
		out.Scope = "jql"
	}
	out.Site = p.SiteID()
	out.Members = jiraMembers(st)
	for _, m := range out.Members {
		if m.JiraSite == out.Site && m.JiraUserID != "" && m.JiraLogin != "" {
			out.Linked++
		}
	}
	return out, nil
}
func (s *Service) JiraUsers(ctx context.Context, search string) ([]domain.JiraUser, error) {
	p, e := s.jiraSource(ctx)
	if e != nil {
		return nil, e
	}
	users, e := p.SearchUsers(ctx, search)
	if e != nil {
		return nil, invalid(e.Error())
	}
	return users, nil
}
func setJiraLink(st *domain.State, memberID, site string, u domain.JiraUser) error {
	found := -1
	for i, m := range st.Members {
		if m.Demo || m.Inactive || m.Source != "bitrix" || m.AccountID != st.TeamSync.AccountID {
			continue
		}
		if m.ID == memberID {
			found = i
		} else if m.JiraSite == site && m.JiraUserID == u.ID {
			return invalid("Этот пользователь Jira уже связан с другим участником команды")
		}
	}
	if found < 0 {
		return invalid("Сотрудник больше не входит в действующую команду")
	}
	m := &st.Members[found]
	m.JiraSite = site
	m.JiraUserID = u.ID
	m.JiraLogin = u.Login
	m.JiraName = u.Name
	return nil
}
func (s *Service) LinkJiraMember(ctx context.Context, memberID, userID string) (domain.JiraTeamStatus, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	p, e := s.jiraSource(ctx)
	if e != nil {
		return domain.JiraTeamStatus{}, e
	}
	if userID == "" || len(userID) > 250 {
		return domain.JiraTeamStatus{}, invalid("Выберите пользователя Jira")
	}
	u, e := p.User(ctx, userID)
	if e != nil {
		return domain.JiraTeamStatus{}, invalid(e.Error())
	}
	e = s.updateCore(ctx, func(st *domain.State) error {
		if st.Settings.Demo || !st.Settings.Modules["jira"] {
			return invalid("Настройки изменились")
		}
		return setJiraLink(st, memberID, p.SiteID(), u)
	})
	if e != nil {
		return domain.JiraTeamStatus{}, e
	}
	return s.JiraTeamStatus(ctx)
}
func (s *Service) MatchJiraTeam(ctx context.Context) (domain.JiraTeamStatus, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	p, e := s.jiraSource(ctx)
	if e != nil {
		return domain.JiraTeamStatus{}, e
	}
	_, e = s.resolveJiraTeam(ctx, p)
	status, readErr := s.JiraTeamStatus(ctx)
	if e != nil {
		return status, e
	}
	return status, readErr
}

// Exact email only: names can collide, while absent/private emails require an explicit choice.
func (s *Service) resolveJiraTeam(ctx context.Context, p ports.TeamTaskSource) ([]domain.Member, error) {
	st, e := s.viewCore(ctx)
	if e != nil {
		return nil, e
	}
	members := jiraMembers(st)
	if len(members) == 0 {
		return nil, invalid("Нет действующей команды из Битрикса. Обновите состав в «Загрузке команды»")
	}
	needEmails := false
	for _, m := range members {
		if (m.JiraSite != p.SiteID() || m.JiraUserID == "") && m.Email == "" {
			needEmails = true
		}
	}
	if needEmails && s.bitrix != nil && s.bitrix.Configured() && st.Settings.Modules["team"] && st.Settings.Modules["bitrix"] {
		if _, e = s.SyncTeam(ctx, false); e != nil {
			return nil, e
		}
		st, e = s.viewCore(ctx)
		if e != nil {
			return nil, e
		}
		members = jiraMembers(st)
	}
	if len(members) == 0 {
		return nil, invalid("Команда пуста; загрузка всех задач Jira не выполняется")
	}
	fingerprint := jiraTeamFingerprint(st)
	links := map[string]domain.JiraUser{}
	missing := []string{}
	for _, m := range members {
		if m.JiraSite == p.SiteID() && m.JiraUserID != "" && m.JiraLogin != "" {
			continue
		}
		email := strings.TrimSpace(m.Email)
		matches := []domain.JiraUser{}
		if email != "" {
			users, err := p.SearchUsers(ctx, email)
			if err != nil {
				return nil, invalid(err.Error())
			}
			for _, u := range users {
				if strings.EqualFold(strings.TrimSpace(u.Email), email) {
					matches = append(matches, u)
				}
			}
		}
		if len(matches) == 1 {
			links[m.ID] = matches[0]
		} else {
			missing = append(missing, m.Name)
		}
	}
	e = s.updateCore(ctx, func(current *domain.State) error {
		if current.Settings.Demo || !current.Settings.Modules["jira"] || jiraTeamFingerprint(*current) != fingerprint {
			return invalid("Состав команды изменился; повторите синхронизацию")
		}
		for id, u := range links {
			if e := setJiraLink(current, id, p.SiteID(), u); e != nil {
				return e
			}
		}
		members = jiraMembers(*current)
		return nil
	})
	if e != nil {
		return nil, e
	}
	if len(missing) > 0 {
		return nil, invalid(fmt.Sprintf("Сопоставьте сотрудников с Jira в разделе интеграций: %s. Загрузка задач не выполнялась", strings.Join(missing, ", ")))
	}
	return members, nil
}

func mergeLegacyJiraMembers(st *domain.State, members []domain.Member) {
	aliases := map[string]string{}
	for _, m := range members {
		aliases["jira-"+m.JiraUserID] = m.ID
		aliases["jira-"+m.JiraLogin] = m.ID
	}
	for i := range st.Tasks {
		if !st.Tasks[i].Demo {
			if id := aliases[st.Tasks[i].AssigneeID]; id != "" {
				st.Tasks[i].AssigneeID = id
			}
		}
	}
	for i := range st.Members {
		m := &st.Members[i]
		if !m.Demo && (m.Source == "jira" || (m.Source == "" && strings.HasPrefix(m.ID, "jira-"))) {
			m.Inactive = true
		}
	}
	// Legacy Jira identities may already have day plans. Preserve their dates and
	// hours when binding to Bitrix; an explicit canonical plan takes precedence.
	canonical := map[[3]string]bool{}
	for _, entry := range st.TeamPlan {
		if !entry.Demo && aliases[entry.MemberID] == "" {
			canonical[[3]string{entry.MemberID, entry.Date, entry.TaskID}] = true
		}
	}
	plans := make([]domain.TeamPlanEntry, 0, len(st.TeamPlan))
	for _, entry := range st.TeamPlan {
		if !entry.Demo {
			if id := aliases[entry.MemberID]; id != "" {
				entry.MemberID = id
				key := [3]string{entry.MemberID, entry.Date, entry.TaskID}
				if canonical[key] {
					continue
				}
				canonical[key] = true
			}
		}
		plans = append(plans, entry)
	}
	st.TeamPlan = plans
}

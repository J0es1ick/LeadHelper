package application

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"strings"
	"time"
)

func (s *Service) teamEnabled(ctx context.Context) (domain.State, error) {
	st, e := s.viewCore(ctx)
	if e != nil {
		return st, e
	}
	if st.Settings.Demo {
		return st, invalid("Состав из Битрикса доступен только в рабочем режиме")
	}
	if e = enabled(&st, "team"); e != nil {
		return st, e
	}
	return st, nil
}
func (s *Service) SetTeamAuto(ctx context.Context, value bool) (domain.TeamSyncStatus, error) {
	if value {
		return s.SyncTeam(ctx, true)
	}
	s.teamMu.Lock()
	defer s.teamMu.Unlock()
	if _, e := s.teamEnabled(ctx); e != nil {
		return domain.TeamSyncStatus{}, e
	}
	var out domain.TeamSyncStatus
	e := s.updateCore(ctx, func(st *domain.State) error {
		if st.Settings.Demo {
			return invalid("Режим изменился")
		}
		st.TeamSync.Enabled = false
		out = st.TeamSync
		return nil
	})
	return out, e
}

// Only a complete snapshot replaces membership. Capacity, IDs and old tasks remain local.
func (s *Service) SyncTeam(ctx context.Context, enableAuto bool) (domain.TeamSyncStatus, error) {
	s.teamMu.Lock()
	defer s.teamMu.Unlock()
	st, e := s.teamEnabled(ctx)
	if e != nil {
		return domain.TeamSyncStatus{}, e
	}
	if e = enabled(&st, "bitrix"); e != nil {
		return st.TeamSync, e
	}
	source, ok := s.bitrix.(ports.TeamSource)
	if !ok || s.bitrix == nil || !s.bitrix.Configured() {
		return st.TeamSync, invalid("Подключите Битрикс24, чтобы загрузить подчинённых")
	}
	account := s.bitrix.AccountID()
	now := time.Now().UTC()
	e = s.updateCore(ctx, func(current *domain.State) error {
		if current.Settings.Demo || !current.Settings.Modules["team"] || !current.Settings.Modules["bitrix"] {
			return invalid("Настройки изменились")
		}
		current.TeamSync.LastAttemptAt = &now
		if enableAuto {
			current.TeamSync.Enabled = true
		}
		return nil
	})
	if e != nil {
		return st.TeamSync, e
	}
	snapshot, fetchErr := source.FetchTeam(ctx)
	if fetchErr == nil {
		if snapshot.AccountID != account || snapshot.ManagerID == "" || len(snapshot.Roots) == 0 {
			fetchErr = errors.New("Не удалось подтвердить отдел руководителя; предыдущий состав сохранён")
		}
		seen := map[string]bool{}
		for _, person := range snapshot.Members {
			if person.ID == "" || strings.TrimSpace(person.Name) == "" || seen[person.ID] {
				fetchErr = errors.New("Битрикс24 вернул некорректный состав; предыдущая команда сохранена")
				break
			}
			seen[person.ID] = true
		}
	}
	// A request timeout must still leave a visible error instead of silently stale membership.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var out domain.TeamSyncStatus
	e = s.updateCore(saveCtx, func(current *domain.State) error {
		if current.Settings.Demo || !current.Settings.Modules["team"] || !current.Settings.Modules["bitrix"] {
			return invalid("Настройки изменились во время обновления команды")
		}
		if s.bitrix.AccountID() != account {
			return invalid("Подключение Битрикса изменилось; повторите обновление команды")
		}
		if fetchErr != nil {
			current.TeamSync.Error = fetchErr.Error()
			out = current.TeamSync
			return nil
		}
		incoming := map[string]domain.TeamPerson{}
		for _, person := range snapshot.Members {
			if person.ID != snapshot.ManagerID {
				incoming[person.ID] = person
			}
		}
		found := map[string]bool{}
		for i := range current.Members {
			m := &current.Members[i]
			if m.Demo || m.Source != "bitrix" {
				continue
			}
			person, exists := incoming[m.ExternalID]
			if m.AccountID != account || !exists {
				m.Inactive = true
				continue
			}
			m.Name = person.Name
			m.Email = person.Email
			m.Role = person.Role
			m.Departments = person.Departments
			m.Inactive = false
			found[m.ExternalID] = true
		}
		for _, person := range snapshot.Members {
			if person.ID == snapshot.ManagerID || found[person.ID] {
				continue
			}
			current.Members = append(current.Members, domain.Member{ID: account + "-user-" + person.ID, Source: "bitrix", ExternalID: person.ID, AccountID: account, Name: person.Name, Email: person.Email, Role: person.Role, Departments: person.Departments, Capacity: 32, Color: "#497968"})
		}
		completed := time.Now().UTC()
		current.TeamSync.AccountID = account
		current.TeamSync.ManagerName = snapshot.ManagerName
		current.TeamSync.Roots = snapshot.Roots
		current.TeamSync.DepartmentCount = len(snapshot.Departments)
		current.TeamSync.MemberCount = len(incoming)
		current.TeamSync.LastSuccessAt = &completed
		current.TeamSync.Error = ""
		out = current.TeamSync
		return nil
	})
	if e != nil {
		return out, e
	}
	if fetchErr != nil {
		return out, invalid(fetchErr.Error())
	}
	return out, nil
}

package integrations

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"sort"
	"strconv"
)

// managedDepartments follows explicit management and parent links, never mere department membership.
func managedDepartments(items []domain.RemoteItem, manager string) ([]domain.TeamDepartment, []domain.TeamDepartment, error) {
	byID := map[string]domain.RemoteItem{}
	managed := map[string]bool{}
	for _, d := range items {
		byID[d.ID] = d
		if d.HeadID == manager && manager != "" {
			managed[d.ID] = true
		}
	}
	if len(managed) == 0 {
		return nil, nil, errors.New("В Битриксе не найден отдел, где вы указаны руководителем. Проверьте структуру компании и учётную запись вебхука")
	}
	roots := []domain.TeamDepartment{}
	for _, d := range items {
		if !managed[d.ID] {
			continue
		}
		nested := false
		seen := map[string]bool{d.ID: true}
		parent := d.Parent
		for parent != "" && parent != "0" {
			if seen[parent] {
				return nil, nil, errors.New("В структуре Битрикса обнаружен цикл подчинения")
			}
			seen[parent] = true
			if managed[parent] {
				nested = true
			}
			p, ok := byID[parent]
			if !ok {
				break
			}
			parent = p.Parent
		}
		if !nested {
			roots = append(roots, domain.TeamDepartment{ID: d.ID, Name: d.Title, Parent: d.Parent})
		}
	}
	included := map[string]bool{}
	for _, d := range roots {
		included[d.ID] = true
	}
	for changed := true; changed; {
		changed = false
		for _, d := range items {
			if !included[d.ID] && included[d.Parent] {
				included[d.ID] = true
				changed = true
			}
		}
	}
	departments := []domain.TeamDepartment{}
	for _, d := range items {
		if included[d.ID] {
			departments = append(departments, domain.TeamDepartment{ID: d.ID, Name: d.Title, Parent: d.Parent})
		}
	}
	return roots, departments, nil
}
func (b *Bitrix) FetchTeam(ctx context.Context) (domain.TeamSnapshot, error) {
	out := domain.TeamSnapshot{AccountID: b.AccountID(), Members: []domain.TeamPerson{}}
	profile, e := b.identity(ctx)
	if e != nil {
		return out, e
	}
	out.ManagerID = str(profile["ID"])
	out.ManagerName = userName(profile)
	if out.ManagerID == "" {
		return out, errors.New("Битрикс24 не вернул пользователя подключения")
	}
	structure, e := b.departments(ctx)
	if e != nil {
		return out, e
	}
	out.Roots, out.Departments, e = managedDepartments(structure.Items, out.ManagerID)
	if e != nil {
		return out, e
	}
	ids := []string{}
	departments := map[string]domain.TeamDepartment{}
	for _, d := range out.Departments {
		ids = append(ids, d.ID)
		departments[d.ID] = d
	}
	cursor := ""
	seenCursors := map[string]bool{}
	seenUsers := map[string]bool{}
	for page := 0; page < 200; page++ {
		start, e := remoteStart(cursor)
		if e != nil {
			return out, e
		}
		var rows []map[string]any
		next, e := b.call(ctx, "user.get", map[string]any{"FILTER": map[string]any{"ACTIVE": true, "USER_TYPE": "employee", "UF_DEPARTMENT": ids}, "SORT": "ID", "ORDER": "ASC", "start": start}, &rows)
		if e != nil {
			return out, e
		}
		for _, row := range rows {
			id := str(row["ID"])
			n, e := strconv.Atoi(id)
			if e != nil || n <= 0 {
				return out, errors.New("Битрикс24 вернул сотрудника без идентификатора")
			}
			if id == out.ManagerID || seenUsers[id] || !bitrixBool(row["ACTIVE"]) {
				continue
			}
			if userType := str(row["USER_TYPE"]); userType != "" && userType != "employee" {
				continue
			}
			linked := []domain.TeamDepartment{}
			seenDept := map[string]bool{}
			for _, did := range bitrixIDs(row["UF_DEPARTMENT"]) {
				if d, ok := departments[did]; ok && !seenDept[did] {
					linked = append(linked, d)
					seenDept[did] = true
				}
			}
			if len(linked) == 0 {
				continue
			}
			seenUsers[id] = true
			name := userName(row)
			if name == "" {
				name = "Сотрудник " + id
			}
			out.Members = append(out.Members, domain.TeamPerson{ID: id, Email: str(row["EMAIL"]), Name: name, Role: str(row["WORK_POSITION"]), Departments: linked})
		}
		if next == "" {
			sort.SliceStable(out.Members, func(i, j int) bool { return out.Members[i].Name < out.Members[j].Name })
			return out, nil
		}
		nextStart, e := remoteStart(next)
		if e != nil || nextStart <= start || seenCursors[next] || len(rows) == 0 {
			return out, errors.New("Битрикс24 вернул повторную или неполную страницу сотрудников")
		}
		seenCursors[next] = true
		cursor = next
	}
	return out, errors.New("Слишком большая команда: состав не обновлён, чтобы не сохранить неполный список")
}

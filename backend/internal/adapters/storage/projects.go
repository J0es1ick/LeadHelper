package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"strings"
	"time"
)

const projectSchema = `CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY,demo INTEGER NOT NULL,name TEXT NOT NULL,state TEXT NOT NULL,search TEXT NOT NULL,data TEXT NOT NULL);CREATE INDEX IF NOT EXISTS projects_page ON projects(demo,state,name,id);CREATE INDEX IF NOT EXISTS projects_name ON projects(demo,name);CREATE TABLE IF NOT EXISTS project_imports(id TEXT PRIMARY KEY,digest TEXT NOT NULL);`

func (r *SQLiteRepository) Projects(ctx context.Context, demo bool, q domain.ProjectQuery) (domain.ProjectPage, error) {
	out := domain.ProjectPage{Items: []domain.Project{}, Page: q.Page, Limit: q.Limit, Counts: map[string]int{}}
	tx, e := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM projects WHERE demo=? AND coalesce(json_extract(data,'$.sourceVisibility'),'')<>''", demo).Scan(&out.HiddenTotal); e != nil {
		return out, e
	}
	visibilityFilter := ""
	if !q.IncludeHidden {
		visibilityFilter = " AND coalesce(json_extract(data,'$.sourceVisibility'),'')=''"
	}
	where := "demo=? AND instr(search,?)>0"
	where += visibilityFilter
	args := []any{demo, strings.ToLower(q.Search)}
	if q.Group != "" {
		where += " AND json_extract(data,'$.group')=?"
		args = append(args, q.Group)
	}
	if q.Unfiled {
		where += " AND coalesce(json_extract(data,'$.group'),'')=''"
	} else if q.Folder != "" {
		where += " AND (json_extract(data,'$.group')=? OR instr(json_extract(data,'$.group'),?)=1)"
		args = append(args, q.Folder, q.Folder+" / ")
	}
	countWhere, countArgs := where, append([]any{}, args...)
	if q.State != "all" {
		where += " AND state=?"
		args = append(args, q.State)
	}
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM projects WHERE "+where, args...).Scan(&out.Total); e != nil {
		return out, e
	}
	groups, e := tx.QueryContext(ctx, "SELECT DISTINCT json_extract(data,'$.group') FROM projects WHERE demo=? AND json_extract(data,'$.group') IS NOT NULL"+visibilityFilter+" ORDER BY 1", demo)
	if e != nil {
		return out, e
	}
	for groups.Next() {
		var g string
		if e = groups.Scan(&g); e != nil {
			groups.Close()
			return out, e
		}
		out.Groups = append(out.Groups, g)
	}
	e = groups.Err()
	groups.Close()
	if e != nil {
		return out, e
	}
	rows, e := tx.QueryContext(ctx, "SELECT state,count(*) FROM projects WHERE "+countWhere+" GROUP BY state", countArgs...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var state string
		var n int
		if e = rows.Scan(&state, &n); e != nil {
			rows.Close()
			return out, e
		}
		out.Counts[state] = n
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.QueryContext(ctx, "SELECT data FROM projects WHERE "+where+" ORDER BY name,id LIMIT ? OFFSET ?", append(args, q.Limit, (q.Page-1)*q.Limit)...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var raw string
		var p domain.Project
		if e = rows.Scan(&raw); e != nil {
			break
		}
		if e = json.Unmarshal([]byte(raw), &p); e != nil {
			break
		}
		p.HistoryCount = len(p.History)
		p.History = nil
		p.Fields = nil
		out.Items = append(out.Items, p)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func (r *SQLiteRepository) ProjectByName(ctx context.Context, demo bool, name string) (domain.Project, error) {
	return r.projectByName(ctx, demo, name, nil)
}
func (r *SQLiteRepository) ProjectByNameInGroup(ctx context.Context, demo bool, name, group string) (domain.Project, error) {
	return r.projectByName(ctx, demo, name, &group)
}
func (r *SQLiteRepository) projectByName(ctx context.Context, demo bool, name string, group *string) (domain.Project, error) {
	where := "demo=? AND name=?"
	args := []any{demo, strings.ToLower(strings.TrimSpace(name))}
	if group != nil {
		where += " AND coalesce(json_extract(data,'$.group'),'')=?"
		args = append(args, *group)
	}
	rows, e := r.db.QueryContext(ctx, "SELECT data FROM projects WHERE "+where+" LIMIT 2", args...)
	if e != nil {
		return domain.Project{}, e
	}
	defer rows.Close()
	var p domain.Project
	n := 0
	for rows.Next() {
		n++
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return p, e
		}
		if e = json.Unmarshal([]byte(raw), &p); e != nil {
			return p, e
		}
	}
	if e = rows.Err(); e != nil {
		return p, e
	}
	if n == 0 {
		return p, sql.ErrNoRows
	}
	if n > 1 {
		return p, fmt.Errorf("Несколько проектов с названием «%s». Уточните названия перед обновлением", name)
	}
	return p, nil
}
func (r *SQLiteRepository) SaveProjects(ctx context.Context, demo bool, projects []domain.Project, requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var raw string
	if e = tx.QueryRowContext(ctx, "SELECT data FROM workspace_shell WHERE id=1").Scan(&raw); e != nil {
		return e
	}
	var st domain.State
	if e = json.Unmarshal([]byte(raw), &st); e != nil {
		return e
	}
	if st.Settings.Demo != demo || !st.Settings.Modules["projects"] {
		return errors.New("Режим или настройки проектов изменились")
	}
	payload, _ := json.Marshal(struct {
		Demo     bool
		Projects []domain.Project
	}{demo, projects})
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	if requestID != "" {
		var existing string
		e = tx.QueryRowContext(ctx, "SELECT digest FROM project_imports WHERE id=?", requestID).Scan(&existing)
		if e == nil {
			if existing != digest {
				return errors.New("Содержимое импорта изменилось. Подготовьте новый предпросмотр")
			}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
	}
	for _, p := range projects {
		p.RemoteVersion = nil
		var current domain.Project
		var owner bool
		e = tx.QueryRowContext(ctx, "SELECT demo,data FROM projects WHERE id=?", p.ID).Scan(&owner, &raw)
		if e == nil {
			if err := json.Unmarshal([]byte(raw), &current); err != nil {
				return err
			}
			if owner != demo || p.Version == 0 || current.Version != p.Version {
				return errors.New("Проект изменён в другой вкладке. Обновите список")
			}
		} else if !errors.Is(e, sql.ErrNoRows) {
			return e
		} else if p.Version != 0 {
			return errors.New("Проект больше не существует")
		}
		if current.ID != "" {
			p.SourceVisibility = current.SourceVisibility
			if requestID != "" && current.Sheet != nil {
				return errors.New("Проект связан с Google Таблицей. Обновите его через синхронизацию или редактор проекта")
			}
			if requestID == "" || p.History == nil {
				p.History = current.History
			}
			if p.Fields == nil {
				p.Fields = current.Fields
			} else {
				if len(p.Fields) != len(current.Fields) {
					return errors.New("Столбцы таблицы изменились. Откройте проект снова")
				}
				for i, f := range p.Fields {
					if f.Key != current.Fields[i].Key || len(f.Value) > 60000 {
						return errors.New("Некорректное поле таблицы")
					}
					p.Fields[i] = current.Fields[i]
					p.Fields[i].Value = f.Value
				}
			}
			p.Sheet = current.Sheet
			if current.Sheet != nil {
				p.Group = current.Group
			}
			p.SyncState = current.SyncState
			p.SyncMessage = current.SyncMessage
		}
		p.HistoryCount = len(p.History)
		if p.Sheet != nil {
			if current.SyncState == "conflict" {
				return errors.New("Сначала разрешите конфликт с таблицей")
			}
			p.SyncState = "pending"
			if _, e = tx.ExecContext(ctx, "INSERT INTO project_sync(id,base) VALUES(?,?) ON CONFLICT(id) DO NOTHING", p.ID, projectJSON(domain.Project{})); e != nil {
				return e
			}
		}
		p.Version++
		p.UpdatedAt = time.Now().UTC()
		data, e := json.Marshal(p)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO projects(id,demo,name,state,search,data) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,state=excluded.state,search=excluded.search,data=excluded.data`, p.ID, demo, strings.ToLower(p.Name), p.State, strings.ToLower(p.Name+" "+p.Owner+" "+p.Status+" "+p.Description), string(data))
		if e != nil {
			return e
		}
	}
	if requestID != "" {
		if _, e = tx.ExecContext(ctx, "INSERT INTO project_imports VALUES(?,?)", requestID, digest); e != nil {
			return e
		}
	}
	return tx.Commit()
}

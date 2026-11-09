package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"reflect"
	"strings"
	"time"
)

const projectSyncSchema = `CREATE TABLE IF NOT EXISTS project_sync(id TEXT PRIMARY KEY,base TEXT NOT NULL,remote TEXT NOT NULL DEFAULT '');CREATE TABLE IF NOT EXISTS project_source(id INTEGER PRIMARY KEY CHECK(id=1),data TEXT NOT NULL);`

func projectJSON(p domain.Project) string { b, _ := json.Marshal(p); return string(b) }
func writeProject(ctx context.Context, tx *sql.Tx, p domain.Project, demo bool) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO projects(id,demo,name,state,search,data) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,state=excluded.state,search=excluded.search,data=excluded.data`, p.ID, demo, strings.ToLower(p.Name), p.State, strings.ToLower(p.Name+" "+p.Owner+" "+p.Status+" "+p.Description+" "+p.Group), projectJSON(p))
	return e
}
func (r *SQLiteRepository) ProjectDetail(ctx context.Context, demo bool, id string) (domain.Project, error) {
	var raw string
	var p domain.Project
	e := r.db.QueryRowContext(ctx, "SELECT data FROM projects WHERE id=? AND demo=?", id, demo).Scan(&raw)
	if e == nil {
		e = json.Unmarshal([]byte(raw), &p)
	}
	if e == nil && p.SyncState == "conflict" {
		var remote string
		if err := r.db.QueryRowContext(ctx, "SELECT remote FROM project_sync WHERE id=?", id).Scan(&remote); err == nil && remote != "" {
			var v domain.Project
			if json.Unmarshal([]byte(remote), &v) == nil {
				p.RemoteVersion = &v
			}
		}
	}
	return p, e
}

// Merge each editable field separately. Concurrent edits to the same field need a decision.
func mergeProject(local, base, remote domain.Project) (domain.Project, bool) {
	merged := remote
	conflict := false
	triples := [][3]*string{{&merged.Name, &local.Name, &base.Name}, {&merged.Status, &local.Status, &base.Status}, {&merged.State, &local.State, &base.State}, {&merged.Owner, &local.Owner, &base.Owner}, {&merged.Description, &local.Description, &base.Description}, {&merged.StartDate, &local.StartDate, &base.StartDate}, {&merged.DueDate, &local.DueDate, &base.DueDate}, {&merged.URL, &local.URL, &base.URL}}
	for _, t := range triples {
		if *t[1] != *t[2] {
			if *t[0] != *t[2] && *t[0] != *t[1] {
				conflict = true
			}
			*t[0] = *t[1]
		}
	}
	if !reflect.DeepEqual(local.Fields, base.Fields) {
		if !reflect.DeepEqual(remote.Fields, base.Fields) && !reflect.DeepEqual(remote.Fields, local.Fields) {
			conflict = true
		}
		merged.Fields = local.Fields
	}
	return merged, conflict
}
func projectChanged(a, b domain.Project) bool {
	return a.Name != b.Name || a.Status != b.Status || a.State != b.State || a.Owner != b.Owner || a.Description != b.Description || a.StartDate != b.StartDate || a.DueDate != b.DueDate || a.URL != b.URL || !reflect.DeepEqual(a.Fields, b.Fields)
}
func (r *SQLiteRepository) MergeProjectSheet(ctx context.Context, book domain.ProjectSheetSnapshot, projects []domain.Project, bootstrap bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var previous domain.ProjectSyncStatus
	var sourceRaw string
	e = tx.QueryRowContext(ctx, "SELECT data FROM project_source WHERE id=1").Scan(&sourceRaw)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if e == nil {
		if e = json.Unmarshal([]byte(sourceRaw), &previous); e != nil {
			return e
		}
	}
	if bootstrap && previous.SpreadsheetID != "" {
		// Repair old imports once, using only verified visibility metadata. Never replay
		// source content or overwrite local edits from an older bootstrap snapshot.
		if previous.SpreadsheetID == book.SpreadsheetID && book.VisibilityKnown && !previous.VisibilityKnown {
			for _, source := range projects {
				var raw string
				e = tx.QueryRowContext(ctx, "SELECT data FROM projects WHERE id=? AND demo=0", source.ID).Scan(&raw)
				if errors.Is(e, sql.ErrNoRows) {
					continue
				}
				if e != nil {
					return e
				}
				var local domain.Project
				if e = json.Unmarshal([]byte(raw), &local); e != nil {
					return e
				}
				if local.Sheet == nil || source.Sheet == nil || local.Sheet.SpreadsheetID != book.SpreadsheetID || local.Sheet.RowID != source.Sheet.RowID {
					continue
				}
				if local.SourceVisibility == source.SourceVisibility {
					continue
				}
				local.SourceVisibility = source.SourceVisibility
				local.Version++
				local.UpdatedAt = time.Now().UTC()
				if e = writeProject(ctx, tx, local, false); e != nil {
					return e
				}
			}
			previous.VisibilityKnown = true
			value, _ := json.Marshal(previous)
			if _, e = tx.ExecContext(ctx, "UPDATE project_source SET data=? WHERE id=1", string(value)); e != nil {
				return e
			}
		}
		return tx.Commit()
	}
	seen := map[string]bool{}
	for _, remote := range projects {
		if seen[remote.ID] {
			return errors.New("Повторяющиеся идентификаторы строк в таблице")
		}
		seen[remote.ID] = true
		local := domain.Project{}
		base := domain.Project{}
		var raw, baseRaw, conflictRaw string
		var demo bool
		e = tx.QueryRowContext(ctx, "SELECT demo,data FROM projects WHERE id=?", remote.ID).Scan(&demo, &raw)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if e == nil {
			if demo {
				return errors.New("Идентификатор проекта занят")
			}
			if e = json.Unmarshal([]byte(raw), &local); e != nil {
				return e
			}
			if local.Sheet == nil || local.Sheet.SpreadsheetID != book.SpreadsheetID {
				return errors.New("Проект принадлежит другому источнику")
			}
		}
		e = tx.QueryRowContext(ctx, "SELECT base,remote FROM project_sync WHERE id=?", remote.ID).Scan(&baseRaw, &conflictRaw)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if e == nil {
			if e = json.Unmarshal([]byte(baseRaw), &base); e != nil {
				return e
			}
		}
		merged := remote
		// Older deployed scripts lack row visibility. Preserve confirmed row flags
		// until a new snapshot explicitly supplies them.
		if !book.VisibilityKnown && remote.SourceVisibility == "" && local.SourceVisibility == "row" {
			remote.SourceVisibility = "row"
			merged = remote
		}
		conflict := false
		if local.ID != "" {
			merged, conflict = mergeProject(local, base, remote)
		}
		if local.SyncState == "conflict" {
			conflict = true
		}
		merged.Version = local.Version
		merged.UpdatedAt = local.UpdatedAt
		merged.SyncState = "synced"
		merged.SyncMessage = ""
		if projectChanged(merged, remote) {
			merged.SyncState = "pending"
		}
		nextConflict := ""
		nextBase := projectJSON(remote)
		if conflict {
			merged.SyncState = "conflict"
			merged.SyncMessage = "Проект изменён и в таблице, и в сервисе. Выберите версию."
			nextConflict = projectJSON(remote)
			nextBase = baseRaw
		}
		if !reflect.DeepEqual(merged, local) {
			merged.Version++
			merged.UpdatedAt = time.Now().UTC()
			if e = writeProject(ctx, tx, merged, false); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO project_sync(id,base,remote) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET base=excluded.base,remote=excluded.remote", remote.ID, nextBase, nextConflict); e != nil {
			return e
		}
	}
	// A deleted source row is retained locally and explicitly marked for review.
	rows, e := tx.QueryContext(ctx, "SELECT data FROM projects WHERE demo=0 AND json_extract(data,'$.sheet.spreadsheetId')=?", book.SpreadsheetID)
	if e != nil {
		return e
	}
	missing := []domain.Project{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			break
		}
		var p domain.Project
		if e = json.Unmarshal([]byte(raw), &p); e != nil {
			break
		}
		if !seen[p.ID] && p.Sheet.Mode != "new" {
			missing = append(missing, p)
		}
	}
	rowErr := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if rowErr != nil {
		return rowErr
	}
	for _, p := range missing {
		p.SyncState = "conflict"
		p.SyncMessage = "Строка отсутствует в таблице. Локальная копия сохранена."
		p.Version++
		if e = writeProject(ctx, tx, p, false); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE project_sync SET remote='' WHERE id=?", p.ID); e != nil {
			return e
		}
	}
	status := domain.ProjectSyncStatus{VisibilityKnown: book.VisibilityKnown || (previous.SpreadsheetID == book.SpreadsheetID && previous.VisibilityKnown), Title: book.Title, SpreadsheetID: book.SpreadsheetID, UpdatedAt: book.FetchedAt}
	rawStatus, _ := json.Marshal(status)
	if _, e = tx.ExecContext(ctx, "INSERT INTO project_source VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", string(rawStatus)); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *SQLiteRepository) PendingProjectChanges(ctx context.Context, book string) ([]domain.ProjectSheetChange, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT p.data,s.base FROM projects p JOIN project_sync s ON p.id=s.id WHERE p.demo=0 AND json_extract(p.data,'$.syncState')='pending' AND json_extract(p.data,'$.sheet.spreadsheetId')=? ORDER BY p.id LIMIT 10`, book)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.ProjectSheetChange{}
	for rows.Next() {
		var a, b string
		if e = rows.Scan(&a, &b); e != nil {
			return nil, e
		}
		var c domain.ProjectSheetChange
		if e = json.Unmarshal([]byte(a), &c.Project); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &c.Base); e != nil {
			return nil, e
		}
		c.OperationID = fmt.Sprintf("%s:%d", c.Project.ID, c.Project.Version)
		out = append(out, c)
	}
	return out, rows.Err()
}
func (r *SQLiteRepository) ProjectSyncStatus(ctx context.Context) (domain.ProjectSyncStatus, error) {
	var out domain.ProjectSyncStatus
	var raw string
	e := r.db.QueryRowContext(ctx, "SELECT data FROM project_source WHERE id=1").Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		e = nil
	}
	if e != nil {
		return out, e
	}
	if raw != "" {
		if e = json.Unmarshal([]byte(raw), &out); e != nil {
			return out, e
		}
	}
	e = r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(json_extract(data,'$.syncState')='pending'),0),COALESCE(SUM(json_extract(data,'$.syncState')='conflict'),0) FROM projects WHERE demo=0`).Scan(&out.Pending, &out.Conflicts)
	return out, e
}
func (r *SQLiteRepository) ProjectSyncError(ctx context.Context, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, e := r.db.ExecContext(ctx, `UPDATE project_source SET data=json_set(data,'$.error',?) WHERE id=1`, message)
	return e
}
func (r *SQLiteRepository) ResolveProjectConflict(ctx context.Context, id string, version int, useRemote bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var raw, remoteRaw string
	if e = tx.QueryRowContext(ctx, "SELECT p.data,s.remote FROM projects p JOIN project_sync s ON p.id=s.id WHERE p.id=? AND p.demo=0", id).Scan(&raw, &remoteRaw); e != nil {
		return e
	}
	var local, remote domain.Project
	if e = json.Unmarshal([]byte(raw), &local); e != nil {
		return e
	}
	if local.Version != version || local.SyncState != "conflict" {
		return errors.New("Проект изменился, откройте его снова")
	}
	if remoteRaw == "" {
		return errors.New("Строка удалена из таблицы. Восстановите её там для продолжения синхронизации")
	}
	if e = json.Unmarshal([]byte(remoteRaw), &remote); e != nil {
		return e
	}
	next := local
	if useRemote {
		next = remote
	} else {
		next.Sheet = remote.Sheet
		next.History = remote.History
		next.HistoryCount = remote.HistoryCount
	}
	next.SyncState = "pending"
	if !projectChanged(next, remote) {
		next.SyncState = "synced"
	}
	next.SyncMessage = ""
	next.Version = local.Version + 1
	next.UpdatedAt = time.Now().UTC()
	if e = writeProject(ctx, tx, next, false); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE project_sync SET base=?,remote='' WHERE id=?", remoteRaw, id); e != nil {
		return e
	}
	return tx.Commit()
}

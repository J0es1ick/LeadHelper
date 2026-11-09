package storage

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"leadhelper/internal/domain"
	"modernc.org/sqlite"
	"strings"
)

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("leadhelper_lower", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		v, _ := args[0].(string)
		return strings.ToLower(v), nil
	})
}

const taskProjection = `SELECT coalesce(json_extract(value,'$.id'),''),coalesce(json_extract(value,'$.demo'),0),
 coalesce(json_extract(value,'$.personal'),0),coalesce(json_extract(value,'$.status'),''),coalesce(json_extract(value,'$.source'),''),
 coalesce(json_extract(value,'$.assigneeId'),''),coalesce(json_extract(value,'$.priority'),''),coalesce(json_extract(value,'$.plannedDate'),''),
 coalesce(json_extract(value,'$.dueDate'),''),coalesce(json_extract(value,'$.updatedAt'),''),
 leadhelper_lower(coalesce(json_extract(value,'$.title'),'')||' '||coalesce(json_extract(value,'$.description'),'')||' '||coalesce(json_extract(value,'$.externalId'),'')||' '||coalesce(json_extract(value,'$.project'),'')),value FROM json_each(%s,'$.tasks') WHERE type='object'`

func (r *SQLiteRepository) initTaskIndex(ctx context.Context) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	schema := `CREATE TABLE IF NOT EXISTS task_rows(id TEXT NOT NULL,demo INTEGER NOT NULL,personal INTEGER NOT NULL,status TEXT NOT NULL,source TEXT NOT NULL,assignee TEXT NOT NULL,priority TEXT NOT NULL,planned_date TEXT NOT NULL,due_date TEXT NOT NULL,updated_at TEXT NOT NULL,search TEXT NOT NULL,data TEXT NOT NULL,PRIMARY KEY(demo,id));
 CREATE INDEX IF NOT EXISTS task_status_page ON task_rows(demo,personal,status,updated_at DESC,id);
 CREATE INDEX IF NOT EXISTS task_assignee_page ON task_rows(demo,personal,assignee,status,updated_at DESC,id);
 CREATE INDEX IF NOT EXISTS task_plan_page ON task_rows(demo,personal,planned_date,id);
 CREATE TABLE IF NOT EXISTS workspace_shell(id INTEGER PRIMARY KEY,data TEXT NOT NULL,revision INTEGER NOT NULL);`
	if _, e = tx.ExecContext(ctx, schema); e != nil {
		return e
	}
	var ready int
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM workspace_shell").Scan(&ready); e != nil {
		return e
	}
	if ready == 0 {
		if _, e = tx.ExecContext(ctx, "DELETE FROM task_rows; INSERT INTO task_rows "+fmt.Sprintf(taskProjection, "(SELECT data FROM core WHERE id=1)")); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO workspace_shell SELECT id,json_remove(data,'$.tasks','$.teamPlan'),1 FROM core`); e != nil {
			return e
		}
	}
	triggers := `CREATE TRIGGER IF NOT EXISTS workspace_shell_updated AFTER UPDATE OF data ON core BEGIN
 UPDATE workspace_shell SET data=json_remove(new.data,'$.tasks','$.teamPlan'),revision=revision+1 WHERE id=new.id; END;
 CREATE TRIGGER IF NOT EXISTS task_rows_updated AFTER UPDATE OF data ON core WHEN json_extract(new.data,'$.tasks') IS NOT json_extract(old.data,'$.tasks') BEGIN
 DELETE FROM task_rows; INSERT INTO task_rows ` + fmt.Sprintf(taskProjection, "new.data") + `; END;`
	if _, e = tx.ExecContext(ctx, triggers); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *SQLiteRepository) ViewShell(ctx context.Context) (domain.State, error) {
	var st domain.State
	var data string
	var revision int64
	e := r.db.QueryRowContext(ctx, "SELECT data,revision FROM workspace_shell WHERE id=1").Scan(&data, &revision)
	if e == nil {
		e = json.Unmarshal([]byte(data), &st)
	}
	st.Revision = fmt.Sprint(revision)
	st.Tasks = []domain.Task{}
	st.Messages = []domain.Message{}
	if e == nil {
		e = reconcileMailNotifications(ctx, r.db, &st)
	}
	return st, e
}
func taskWhere(q domain.TaskQuery) (string, []any) {
	where := "demo=?"
	args := []any{q.Demo}
	if len(q.Sources) == 0 {
		where += " AND 0=1"
	} else {
		where += " AND source IN (" + strings.TrimRight(strings.Repeat("?,", len(q.Sources)), ",") + ")"
		for _, s := range q.Sources {
			args = append(args, s)
		}
	}
	if q.Personal != "all" {
		where += " AND personal=?"
		args = append(args, q.Personal == "true")
	}
	switch q.Status {
	case "", "all":
	case "active":
		where += " AND status<>'done'"
	case "actionable":
		where += " AND status IN ('todo','progress')"
	default:
		where += " AND status=?"
		args = append(args, q.Status)
	}
	for _, pair := range [][2]string{{"source", q.Source}, {"priority", q.Priority}} {
		if pair[1] != "" && pair[1] != "all" {
			where += " AND " + pair[0] + "=?"
			args = append(args, pair[1])
		}
	}
	if q.Assignee != "all" {
		where += " AND assignee=?"
		args = append(args, q.Assignee)
	}
	if q.From == "none" {
		where += " AND planned_date=''"
	} else if q.From != "" {
		where += " AND planned_date>=? AND planned_date<=?"
		args = append(args, q.From, q.To)
	}
	if q.Search != "" {
		where += " AND instr(search,?)>0"
		args = append(args, strings.ToLower(q.Search))
	}
	return where, args
}
func (r *SQLiteRepository) TaskPage(ctx context.Context, q domain.TaskQuery) (domain.TaskPage, error) {
	out := domain.TaskPage{Items: []domain.Task{}, Page: q.Page, Limit: q.Limit}
	where, args := taskWhere(q)
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM task_rows WHERE "+where, args...).Scan(&out.Total); e != nil {
		return out, e
	}
	out.Pages = max(1, (out.Total+q.Limit-1)/q.Limit)
	out.Page = min(max(1, q.Page), out.Pages)
	order := "updated_at DESC,id ASC"
	if q.Sort == "due" {
		order = "CASE WHEN due_date='' THEN '9999' ELSE due_date END ASC,id ASC"
	}
	rows, e := tx.QueryContext(ctx, "SELECT json_remove(data,'$.description') FROM task_rows WHERE "+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", append(args, q.Limit, (out.Page-1)*q.Limit)...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var t domain.Task
		if e = rows.Scan(&raw); e != nil {
			return out, e
		}
		if e = json.Unmarshal([]byte(raw), &t); e != nil {
			return out, e
		}
		out.Items = append(out.Items, t)
	}
	return out, rows.Err()
}
func (r *SQLiteRepository) Task(ctx context.Context, id string, demo bool) (domain.Task, error) {
	var t domain.Task
	var raw string
	e := r.db.QueryRowContext(ctx, "SELECT data FROM task_rows WHERE demo=? AND id=?", demo, id).Scan(&raw)
	if e == nil {
		e = json.Unmarshal([]byte(raw), &t)
	}
	return t, e
}
func (r *SQLiteRepository) TaskMetrics(ctx context.Context, q domain.TaskQuery, today string) (domain.TaskMetrics, error) {
	out := domain.TaskMetrics{Statuses: map[string]int{}, MemberTotals: map[string]int{}, MemberActive: map[string]int{}, MemberHours: map[string]float64{}}
	q.Personal = "false"
	q.Status = "all"
	where, args := taskWhere(q)
	rows, e := r.db.QueryContext(ctx, "SELECT status,assignee,count(*),sum(CASE WHEN status<>'done' AND due_date<>'' AND due_date<? THEN 1 ELSE 0 END) FROM task_rows WHERE "+where+" GROUP BY status,assignee", append([]any{today}, args...)...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var status, id string
		var count, late int
		if e = rows.Scan(&status, &id, &count, &late); e != nil {
			rows.Close()
			return out, e
		}
		out.Statuses[status] += count
		out.MemberTotals[id] += count
		out.Overdue += late
		if status != "done" {
			out.MemberActive[id] += count
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	q.Personal = "true"
	q.From = today
	q.To = today
	where, args = taskWhere(q)
	e = r.db.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(CASE WHEN status='done' THEN 1 ELSE 0 END),0) FROM task_rows WHERE "+where, args...).Scan(&out.PersonalTotal, &out.PersonalDone)
	return out, e
}
func (r *SQLiteRepository) PlanEntries(ctx context.Context, demo bool, from, to, member string) ([]domain.TeamPlanEntry, error) {
	out := []domain.TeamPlanEntry{}
	query := `SELECT p.value FROM core,json_each(core.data,'$.teamPlan') p WHERE coalesce(json_extract(p.value,'$.demo'),0)=? AND json_extract(p.value,'$.date')>=? AND json_extract(p.value,'$.date')<=?`
	args := []any{demo, from, to}
	if member != "" {
		query += ` AND json_extract(p.value,'$.memberId')=?`
		args = append(args, member)
	}
	rows, e := r.db.QueryContext(ctx, query, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var item domain.TeamPlanEntry
		if e = rows.Scan(&raw); e != nil {
			return out, e
		}
		if e = json.Unmarshal([]byte(raw), &item); e != nil {
			return out, e
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

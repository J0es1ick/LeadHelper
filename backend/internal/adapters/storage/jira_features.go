package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"leadhelper/internal/domain"
	"time"
)

const jiraFeatureSchema = `
CREATE TABLE IF NOT EXISTS jira_writes(site TEXT NOT NULL, request_id TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY(site,request_id));
CREATE TABLE IF NOT EXISTS jira_worklogs(scope TEXT NOT NULL, id TEXT NOT NULL, member TEXT NOT NULL, date TEXT NOT NULL, seconds INTEGER NOT NULL, data TEXT NOT NULL, PRIMARY KEY(scope,id));
CREATE INDEX IF NOT EXISTS jira_worklog_days ON jira_worklogs(scope,date,member);
CREATE TABLE IF NOT EXISTS jira_worklog_ranges(scope TEXT NOT NULL, first TEXT NOT NULL, last TEXT NOT NULL, synced TEXT NOT NULL, PRIMARY KEY(scope,first,last));
`

func (r *SQLiteRepository) JiraWrite(ctx context.Context, site, id string) (domain.JiraWriteRecord, error) {
	var v domain.JiraWriteRecord
	var raw string
	e := r.db.QueryRowContext(ctx, "SELECT data FROM jira_writes WHERE site=? AND request_id=?", site, id).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return v, nil
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal([]byte(raw), &v)
	return v, e
}
func (r *SQLiteRepository) PutJiraWrite(ctx context.Context, site, id string, v domain.JiraWriteRecord) error {
	raw, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = r.db.ExecContext(ctx, "INSERT INTO jira_writes VALUES(?,?,?) ON CONFLICT(site,request_id) DO UPDATE SET data=excluded.data", site, id, string(raw))
	return e
}
func (r *SQLiteRepository) ReplaceWorklogs(ctx context.Context, scope, from, to string, rows []domain.Worklog, at time.Time) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "DELETE FROM jira_worklogs WHERE scope=? AND date>=? AND date<=?", scope, from, to); e != nil {
		return e
	}
	for _, v := range rows {
		raw, e := json.Marshal(v)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO jira_worklogs VALUES(?,?,?,?,?,?) ON CONFLICT(scope,id) DO UPDATE SET member=excluded.member,date=excluded.date,seconds=excluded.seconds,data=excluded.data", scope, v.ID, v.MemberID, v.Date, v.Seconds, string(raw)); e != nil {
			return e
		}
	}
	// An overlapping refresh invalidates previous range completeness markers.
	if _, e = tx.ExecContext(ctx, "DELETE FROM jira_worklog_ranges WHERE scope=? AND first<=? AND last>=?", scope, to, from); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO jira_worklog_ranges VALUES(?,?,?,?)", scope, from, to, at.UTC().Format(time.RFC3339Nano)); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *SQLiteRepository) WorklogWeek(ctx context.Context, scope, from, to string) (domain.WorklogWeek, error) {
	out := domain.WorklogWeek{Cells: []domain.WorklogCell{}, Timezone: "Europe/Moscow"}
	tx, e := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	var stamp string
	e = tx.QueryRowContext(ctx, "SELECT synced FROM jira_worklog_ranges WHERE scope=? AND first<=? AND last>=? ORDER BY synced DESC LIMIT 1", scope, from, to).Scan(&stamp)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	if e == nil {
		t, e := time.Parse(time.RFC3339Nano, stamp)
		if e != nil {
			return out, e
		}
		out.SyncedAt = &t
	}
	// Sum repeated entries for the same issue within one employee/day. Window
	// totals cover ALL issues; only bounded task summaries are sent to the UI.
	rows, e := tx.QueryContext(ctx, `WITH grouped AS (
 SELECT member,date,coalesce(nullif(json_extract(data,'$.issueKey'),''),id) issue_key,
 coalesce(max(json_extract(data,'$.title')),'') title,sum(seconds) seconds,count(*) entries
 FROM jira_worklogs WHERE scope=? AND date>=? AND date<=? GROUP BY member,date,issue_key
), ranked AS (
 SELECT *,sum(seconds) OVER cell total_seconds,sum(entries) OVER cell total_entries,
 count(*) OVER cell task_count,row_number() OVER (PARTITION BY member,date ORDER BY seconds DESC,issue_key ASC) position
 FROM grouped WINDOW cell AS (PARTITION BY member,date)
) SELECT member,date,total_seconds,total_entries,task_count,issue_key,title,seconds,entries
 FROM ranked WHERE position<=10 ORDER BY date,member,position`, scope, from, to)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.WorklogCell
		var task domain.WorklogTask
		if e = rows.Scan(&c.MemberID, &c.Date, &c.Seconds, &c.Count, &c.TaskCount, &task.IssueKey, &task.Title, &task.Seconds, &task.Count); e != nil {
			return out, e
		}
		if len(out.Cells) == 0 || out.Cells[len(out.Cells)-1].MemberID != c.MemberID || out.Cells[len(out.Cells)-1].Date != c.Date {
			c.Tasks = []domain.WorklogTask{}
			out.Cells = append(out.Cells, c)
		}
		last := &out.Cells[len(out.Cells)-1]
		last.Tasks = append(last.Tasks, task)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if e = rows.Close(); e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func (r *SQLiteRepository) WorklogDay(ctx context.Context, scope, member, date string, page, limit int) (domain.WorklogPage, error) {
	out := domain.WorklogPage{Items: []domain.Worklog{}, Page: page, Limit: limit}
	if e := r.db.QueryRowContext(ctx, "SELECT count(*) FROM jira_worklogs WHERE scope=? AND member=? AND date=?", scope, member, date).Scan(&out.Total); e != nil {
		return out, e
	}
	out.Pages = max(1, (out.Total+limit-1)/limit)
	out.Page = min(max(1, page), out.Pages)
	rows, e := r.db.QueryContext(ctx, "SELECT data FROM jira_worklogs WHERE scope=? AND member=? AND date=? ORDER BY id DESC LIMIT ? OFFSET ?", scope, member, date, limit, (out.Page-1)*limit)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var v domain.Worklog
		if e = rows.Scan(&raw); e != nil {
			return out, e
		}
		if e = json.Unmarshal([]byte(raw), &v); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}

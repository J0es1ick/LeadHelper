package storage

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

var replyPrefix = regexp.MustCompile(`(?i)^(\s*(re|fw|fwd|отв|пересл)(\[\d+\])?\s*:\s*)+`)
var messageIDs = regexp.MustCompile(`<[^<>\s]+>`)

func threadKeys(m domain.Message) (string, string, string) {
	scope := fmt.Sprintf("%t:%s", m.Demo, m.AccountID)
	sub := strings.ToLower(strings.TrimSpace(replyPrefix.ReplaceAllString(m.Subject, "")))
	people := map[string]bool{}
	for _, field := range []string{m.From, m.To, m.CC} {
		if addrs, e := mail.ParseAddressList(field); e == nil {
			for _, a := range addrs {
				people[strings.ToLower(a.Address)] = true
			}
		}
	}
	list := []string{}
	for p := range people {
		list = append(list, p)
	}
	sort.Strings(list)
	return scope, sub, strings.Join(list, ",")
}
func (r *SQLiteRepository) putMessage(ctx context.Context, tx *sql.Tx, m domain.Message, changed map[string]bool) error {
	var oldThread string
	var oldRow int64
	err := tx.QueryRowContext(ctx, "SELECT thread_id,rowid FROM messages WHERE id=?", m.ID).Scan(&oldThread, &oldRow)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	scope, sub, people := threadKeys(m)
	thread := m.ID
	if oldThread != "" {
		thread = oldThread
		changed[oldThread] = true
	}
	refs := messageIDs.FindAllString(m.MessageID+" "+m.InReplyTo+" "+m.References, -1)
	linked := map[string]bool{}
	if oldThread != "" {
		linked[oldThread] = true
	}
	if m.Folder != "drafts" {
		for _, ref := range refs {
			var other string
			e := tx.QueryRowContext(ctx, "SELECT thread_id FROM mail_refs WHERE scope=? AND ref=?", scope, ref).Scan(&other)
			if e == nil {
				linked[other] = true
			} else if !errors.Is(e, sql.ErrNoRows) {
				return e
			}
		}
		if len(linked) == 0 && m.InReplyTo == "" && m.References == "" && replyPrefix.MatchString(m.Subject) && len(sub) > 2 && people != "" {
			var other string
			e := tx.QueryRowContext(ctx, `SELECT thread_id FROM messages WHERE scope=? AND subject_key=? AND participants=? AND stamp<? AND stamp>? AND folder<>'drafts' ORDER BY stamp DESC LIMIT 1`, scope, sub, people, m.Date.UnixMicro(), m.Date.Add(-30*24*time.Hour).UnixMicro()).Scan(&other)
			if e == nil {
				linked[other] = true
			} else if !errors.Is(e, sql.ErrNoRows) {
				return e
			}
		}
		keys := []string{}
		for k := range linked {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			thread = keys[0]
		}
		for other := range linked {
			if other == thread {
				continue
			}
			if _, err = tx.ExecContext(ctx, "UPDATE messages SET thread_id=? WHERE thread_id=?", thread, other); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE mail_refs SET thread_id=? WHERE thread_id=?", thread, other); err != nil {
				return err
			}
			changed[other] = true
		}
		for _, ref := range refs {
			if _, err = tx.ExecContext(ctx, "INSERT INTO mail_refs(scope,ref,thread_id) VALUES(?,?,?) ON CONFLICT(scope,ref) DO UPDATE SET thread_id=excluded.thread_id", scope, ref, thread); err != nil {
				return err
			}
		}
	}
	changed[thread] = true
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	brief, err := json.Marshal(summary(m))
	if err != nil {
		return err
	}
	names := []string{"id", "demo", "folder", "account", "source_folder", "uid_validity", "external_id", "content_hash", "message_id", "thread_id", "stamp", "is_read", "starred", "has_files", "format_version", "scope", "subject_key", "participants", "summary", "data"}
	args := []any{m.ID, m.Demo, m.Folder, m.AccountID, m.SourceFolder, m.UIDValidity, m.ExternalID, m.ContentHash, m.MessageID, thread, m.Date.UnixMicro(), m.Read, m.Starred, len(m.Attachments) > 0, m.FormatVersion, scope, sub, people, string(brief), string(data)}
	sets := []string{}
	for _, n := range names[1:] {
		sets = append(sets, n+"=excluded."+n)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO messages("+strings.Join(names, ",")+") VALUES("+strings.TrimRight(strings.Repeat("?,", len(args)), ",")+") ON CONFLICT(id) DO UPDATE SET "+strings.Join(sets, ","), args...)
	if err != nil {
		return err
	}
	if oldRow != 0 {
		if _, err = tx.ExecContext(ctx, "DELETE FROM mail_fts WHERE rowid=?", oldRow); err != nil {
			return err
		}
	}
	var row int64
	if err = tx.QueryRowContext(ctx, "SELECT rowid FROM messages WHERE id=?", m.ID).Scan(&row); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO mail_fts(rowid,id,text) VALUES(?,?,?)", row, m.ID, strings.Join([]string{m.Subject, m.FromName, m.From, m.To, m.CC, m.Body}, " ")); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM mail_files WHERE message_id=?", m.ID); err != nil {
		return err
	}
	for _, a := range m.Attachments {
		if a.BlobKey != "" {
			if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO mail_files VALUES(?,?,?)", m.ID, a.BlobKey, m.Demo); err != nil {
				return err
			}
		}
	}
	return nil
}
func rebuildThread(ctx context.Context, tx *sql.Tx, id string) error {
	if _, e := tx.ExecContext(ctx, "DELETE FROM thread_folders WHERE thread_id=?", id); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, "DELETE FROM threads WHERE id=?", id); e != nil {
		return e
	}
	for _, trash := range []bool{false, true} {
		filter := "thread_id=? AND (folder='trash')=?"
		var demo bool
		var latest string
		var stamp int64
		e := tx.QueryRowContext(ctx, "SELECT demo,id,stamp FROM messages WHERE "+filter+" ORDER BY stamp DESC,id DESC LIMIT 1", id, trash).Scan(&demo, &latest, &stamp)
		if errors.Is(e, sql.ErrNoRows) {
			continue
		}
		if e != nil {
			return e
		}
		var count, unread int
		var star, files bool
		e = tx.QueryRowContext(ctx, `SELECT count(DISTINCT CASE WHEN message_id='' THEN id ELSE message_id END),count(DISTINCT CASE WHEN is_read=0 THEN CASE WHEN message_id='' THEN id ELSE message_id END END),max(starred),max(has_files) FROM messages WHERE `+filter, id, trash).Scan(&count, &unread, &star, &files)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO threads VALUES(?,?,?,?,?,?,?,?,?)", id, trash, demo, latest, stamp, count, unread, star, files); e != nil {
			return e
		}
		rows, e := tx.QueryContext(ctx, "SELECT folder,count(DISTINCT CASE WHEN is_read=0 THEN CASE WHEN message_id='' THEN id ELSE message_id END END) FROM messages WHERE "+filter+" GROUP BY folder", id, trash)
		if e != nil {
			return e
		}
		folders := map[string]int{}
		for rows.Next() {
			var f string
			var n int
			if e = rows.Scan(&f, &n); e != nil {
				rows.Close()
				return e
			}
			folders[f] = n
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if star && !trash {
			folders["starred"] = unread
		}
		if !trash {
			folders["all"] = unread
		}
		for f, n := range folders {
			if _, e = tx.ExecContext(ctx, "INSERT INTO thread_folders VALUES(?,?,?,?,?,?)", id, f, demo, trash, stamp, n); e != nil {
				return e
			}
		}
	}
	return nil
}

type mailCursor struct {
	Stamp int64  `json:"s"`
	ID    string `json:"i"`
	Scope string `json:"q"`
}

func cursorString(stamp int64, id, scope string) string {
	b, _ := json.Marshal(mailCursor{stamp, id, scope})
	return base64.RawURLEncoding.EncodeToString(b)
}
func readCursor(value, scope string) (mailCursor, error) {
	var c mailCursor
	if value == "" {
		return c, nil
	}
	b, e := base64.RawURLEncoding.DecodeString(value)
	if e != nil || len(b) > 2048 {
		return c, fmt.Errorf("Некорректная страница")
	}
	if e = json.Unmarshal(b, &c); e != nil || c.ID == "" || c.Scope != scope {
		return c, fmt.Errorf("Некорректная страница")
	}
	return c, nil
}
func pageLimit(n int) int {
	if n <= 0 {
		return 25
	}
	if n > 100 {
		return 100
	}
	return n
}
func ftsQuery(value string) string {
	words := strings.FieldsFunc(value, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if len(words) > 12 {
		words = words[:12]
	}
	for i, w := range words {
		words[i] = `"` + w + `"*`
	}
	return strings.Join(words, " AND ")
}
func scanThread(row interface{ Scan(...any) error }) (domain.MailThread, int64, error) {
	var t domain.MailThread
	var data string
	var stamp int64
	err := row.Scan(&t.ID, &data, &t.Count, &t.Unread, &t.Starred, &t.HasAttachments, &stamp)
	if err == nil {
		err = json.Unmarshal([]byte(data), &t.Latest)
	}
	return t, stamp, err
}

const threadColumns = "t.id,m.summary,t.count,t.unread,t.starred,t.has_files,t.stamp"

func (r *SQLiteRepository) MailPage(ctx context.Context, demo bool, q domain.MailQuery) (domain.MailPage, error) {
	out := domain.MailPage{Items: []domain.MailThread{}, Limit: pageLimit(q.Limit)}
	scope := fmt.Sprintf("%t:%s:%s", demo, q.Folder, q.Search)
	c, e := readCursor(q.Cursor, scope)
	if e != nil {
		return out, e
	}
	query := "SELECT " + threadColumns + " FROM thread_folders f JOIN threads t ON t.id=f.thread_id AND t.trash=f.trash JOIN messages m ON m.id=t.latest_id WHERE f.demo=? AND f.folder=?"
	args := []any{demo, q.Folder}
	if q.Search != "" {
		search := ftsQuery(q.Search)
		if search == "" {
			return out, nil
		}
		query += ` AND f.thread_id IN (SELECT sm.thread_id FROM mail_fts JOIN messages sm ON sm.rowid=mail_fts.rowid WHERE mail_fts MATCH ? AND sm.demo=? AND (sm.folder='trash')=?)`
		args = append(args, search, demo, q.Folder == "trash")
	}
	if c.ID != "" {
		query += " AND (f.stamp,f.thread_id)<(?,?)"
		args = append(args, c.Stamp, c.ID)
	}
	query += " ORDER BY f.stamp DESC,f.thread_id DESC LIMIT ?"
	args = append(args, out.Limit+1)
	rows, e := r.db.QueryContext(ctx, query, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	var stamp int64
	for rows.Next() {
		t, s, e := scanThread(rows)
		if e != nil {
			return out, e
		}
		if len(out.Items) == out.Limit {
			last := out.Items[len(out.Items)-1]
			out.NextCursor = cursorString(stamp, last.ID, scope)
			break
		}
		stamp = s
		out.Items = append(out.Items, t)
	}
	return out, rows.Err()
}
func (r *SQLiteRepository) MailThread(ctx context.Context, demo bool, id string, q domain.MailQuery) (domain.MessagePage, error) {
	out := domain.MessagePage{Items: []domain.Message{}}
	trash := q.Folder == "trash"
	t, _, e := scanThread(r.db.QueryRowContext(ctx, "SELECT "+threadColumns+" FROM threads t JOIN messages m ON m.id=t.latest_id WHERE t.id=? AND t.demo=? AND t.trash=?", id, demo, trash))
	if errors.Is(e, sql.ErrNoRows) {
		var actual string
		if e = r.db.QueryRowContext(ctx, "SELECT thread_id FROM messages WHERE id=? AND demo=?", id, demo).Scan(&actual); e != nil || actual == id {
			return out, sql.ErrNoRows
		}
		return r.MailThread(ctx, demo, actual, q)
	}
	if e != nil {
		return out, e
	}
	out.Thread = t
	scope := fmt.Sprintf("%t:%s:%t", demo, t.ID, trash)
	c, e := readCursor(q.Cursor, scope)
	if e != nil {
		return out, e
	}
	query := `SELECT m.summary,m.stamp FROM messages m WHERE m.demo=? AND m.thread_id=? AND (m.folder='trash')=? AND (m.message_id='' OR m.id=(SELECT max(copy.id) FROM messages copy WHERE copy.demo=m.demo AND copy.thread_id=m.thread_id AND copy.message_id=m.message_id AND (copy.folder='trash')=?))`
	args := []any{demo, t.ID, trash, trash}
	if c.ID != "" {
		query += " AND (m.stamp,m.id)<(?,?)"
		args = append(args, c.Stamp, c.ID)
	}
	limit := pageLimit(q.Limit)
	query += " ORDER BY m.stamp DESC,m.id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, e := r.db.QueryContext(ctx, query, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	var stamp int64
	for rows.Next() {
		var data string
		var next int64
		if e = rows.Scan(&data, &next); e != nil {
			return out, e
		}
		if len(out.Items) == limit {
			out.NextCursor = cursorString(stamp, out.Items[len(out.Items)-1].ID, scope)
			break
		}
		var m domain.Message
		if e = json.Unmarshal([]byte(data), &m); e != nil {
			return out, e
		}
		stamp = next
		out.Items = append(out.Items, m)
	}
	return out, rows.Err()
}
func (r *SQLiteRepository) MailStats(ctx context.Context, demo bool) (domain.MailStats, error) {
	out := domain.MailStats{Folders: map[string]int{}}
	rows, e := r.db.QueryContext(ctx, "SELECT folder,count,unread FROM folder_stats WHERE demo=?", demo)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var folder string
		var count, unread int
		if e = rows.Scan(&folder, &count, &unread); e != nil {
			rows.Close()
			return out, e
		}
		out.Folders[folder] = count
		if folder == "inbox" {
			out.Unread = unread
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	e = r.db.QueryRowContext(ctx, "SELECT revision FROM core WHERE id=1").Scan(&out.Revision)
	return out, e
}
func (r *SQLiteRepository) PatchThread(ctx context.Context, demo bool, id, folder string, patch domain.MailPatch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var coreData string
	if e = tx.QueryRowContext(ctx, "SELECT data FROM core WHERE id=1").Scan(&coreData); e != nil {
		return e
	}
	var core domain.State
	if e = json.Unmarshal([]byte(coreData), &core); e != nil {
		return e
	}
	if e = reconcileMailNotifications(ctx, tx, &core); e != nil {
		return e
	}
	if core.Settings.Demo != demo || !core.Settings.Modules["mail"] {
		return fmt.Errorf("Почта отключена или режим изменился")
	}
	var canonical string
	e = tx.QueryRowContext(ctx, "SELECT id FROM threads WHERE id=? AND demo=? AND trash=?", id, demo, folder == "trash").Scan(&canonical)
	if errors.Is(e, sql.ErrNoRows) {
		e = tx.QueryRowContext(ctx, "SELECT thread_id FROM messages WHERE id=? AND demo=?", id, demo).Scan(&canonical)
	}
	if e != nil {
		return e
	}
	id = canonical
	where := "demo=? AND thread_id=? AND (folder='trash')=?"
	args := []any{demo, id, folder == "trash"}
	var count, sending int
	if e = tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(json_extract(data,'$.delivery')='sending'),0) FROM messages WHERE "+where, args...).Scan(&count, &sending); e != nil {
		return e
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	if sending > 0 {
		return fmt.Errorf("Дождитесь завершения отправки")
	}
	sets := []string{}
	values := []any{}
	jsonArgs := []string{}
	jsonValues := []any{}
	if patch.Folder != "" {
		sets = append(sets, "folder=?")
		values = append(values, patch.Folder)
		jsonArgs = append(jsonArgs, "'$.folder',?")
		jsonValues = append(jsonValues, patch.Folder)
	}
	for _, p := range []struct {
		col, key string
		value    *bool
	}{{"is_read", "read", patch.Read}, {"starred", "starred", patch.Starred}} {
		if p.value != nil {
			sets = append(sets, p.col+"=?")
			values = append(values, *p.value)
			jsonArgs = append(jsonArgs, "'$.'||?,json(?)")
			jsonValues = append(jsonValues, p.key, fmt.Sprint(*p.value))
		}
	}
	if len(sets) == 0 {
		return nil
	}
	expr := strings.Join(jsonArgs, ",")
	sets = append(sets, "data=json_set(data,"+expr+")", "summary=json_set(summary,"+expr+")")
	values = append(values, jsonValues...)
	values = append(values, jsonValues...)
	values = append(values, args...)
	if _, e = tx.ExecContext(ctx, "UPDATE messages SET "+strings.Join(sets, ",")+" WHERE "+where, values...); e != nil {
		return e
	}
	if e = rebuildThread(ctx, tx, id); e != nil {
		return e
	}
	if e = reconcileMailNotifications(ctx, tx, &core); e != nil {
		return e
	}
	encoded, e := json.Marshal(core)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE core SET data=?,revision=revision+1 WHERE id=1", string(encoded)); e != nil {
		return e
	}
	return tx.Commit()
}

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SQLiteRepository stores mail separately from the small workspace document.
// UI queries never deserialize the whole mailbox. The legacy JSON is retained.
type SQLiteRepository struct {
	db *sql.DB
	mu sync.Mutex
}

const mailSchema = `
CREATE TABLE IF NOT EXISTS core(id INTEGER PRIMARY KEY CHECK(id=1), data TEXT NOT NULL, revision INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS messages(id TEXT PRIMARY KEY, demo INTEGER NOT NULL, folder TEXT NOT NULL, account TEXT NOT NULL, source_folder TEXT NOT NULL, uid_validity INTEGER NOT NULL, external_id TEXT NOT NULL, content_hash TEXT NOT NULL, message_id TEXT NOT NULL, thread_id TEXT NOT NULL, stamp INTEGER NOT NULL, is_read INTEGER NOT NULL, starred INTEGER NOT NULL, has_files INTEGER NOT NULL, format_version INTEGER NOT NULL, scope TEXT NOT NULL, subject_key TEXT NOT NULL, participants TEXT NOT NULL, summary TEXT NOT NULL, data TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS mail_thread ON messages(demo,thread_id,stamp DESC,id DESC);
CREATE INDEX IF NOT EXISTS mail_copies ON messages(demo,thread_id,message_id,id);
CREATE INDEX IF NOT EXISTS mail_external ON messages(account,source_folder,external_id);
CREATE INDEX IF NOT EXISTS mail_hash ON messages(account,source_folder,content_hash);
CREATE INDEX IF NOT EXISTS mail_pending ON messages(format_version,id);
CREATE INDEX IF NOT EXISTS mail_legacy_subject ON messages(scope,subject_key,participants,stamp DESC);
CREATE TABLE IF NOT EXISTS mail_refs(scope TEXT NOT NULL, ref TEXT NOT NULL, thread_id TEXT NOT NULL, PRIMARY KEY(scope,ref));
CREATE INDEX IF NOT EXISTS refs_thread ON mail_refs(thread_id);
CREATE TABLE IF NOT EXISTS mail_files(message_id TEXT NOT NULL, blob_key TEXT NOT NULL, demo INTEGER NOT NULL, PRIMARY KEY(message_id,blob_key));
CREATE INDEX IF NOT EXISTS files_owned ON mail_files(demo,blob_key);
CREATE TABLE IF NOT EXISTS threads(id TEXT NOT NULL, trash INTEGER NOT NULL, demo INTEGER NOT NULL, latest_id TEXT NOT NULL, stamp INTEGER NOT NULL, count INTEGER NOT NULL, unread INTEGER NOT NULL, starred INTEGER NOT NULL, has_files INTEGER NOT NULL, PRIMARY KEY(id,trash));
CREATE TABLE IF NOT EXISTS thread_folders(thread_id TEXT NOT NULL, folder TEXT NOT NULL, demo INTEGER NOT NULL, trash INTEGER NOT NULL, stamp INTEGER NOT NULL, unread INTEGER NOT NULL, PRIMARY KEY(thread_id,folder));
CREATE INDEX IF NOT EXISTS mail_folder_page ON thread_folders(demo,folder,stamp DESC,thread_id DESC);
CREATE TABLE IF NOT EXISTS folder_stats(demo INTEGER NOT NULL,folder TEXT NOT NULL,count INTEGER NOT NULL,unread INTEGER NOT NULL,PRIMARY KEY(demo,folder));
CREATE TRIGGER IF NOT EXISTS folder_added AFTER INSERT ON thread_folders BEGIN INSERT INTO folder_stats VALUES(new.demo,new.folder,1,new.unread) ON CONFLICT(demo,folder) DO UPDATE SET count=count+1,unread=unread+new.unread; END;
CREATE TRIGGER IF NOT EXISTS folder_removed AFTER DELETE ON thread_folders BEGIN UPDATE folder_stats SET count=count-1,unread=unread-old.unread WHERE demo=old.demo AND folder=old.folder; END;
CREATE VIRTUAL TABLE IF NOT EXISTS mail_fts USING fts5(id UNINDEXED, text, tokenize='unicode61', prefix='2 3');
`

func OpenSQLite(path, legacy string, seed domain.State) (*SQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.ToSlash(path)+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	r := &SQLiteRepository{db: db}
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()
	if _, err = db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, err
	}
	if _, err = db.Exec(mailSchema); err != nil {
		return nil, err
	}
	if _, err = db.Exec(jiraFeatureSchema); err != nil {
		return nil, err
	}
	if _, err = db.Exec(projectSchema); err != nil {
		return nil, err
	}
	if _, err = db.Exec(projectSyncSchema); err != nil {
		return nil, err
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM core").Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		raw, e := os.ReadFile(legacy)
		if e == nil {
			if err = json.Unmarshal(raw, &seed); err != nil {
				return nil, fmt.Errorf("legacy database: %w", err)
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return nil, e
		}
		if seed.Version != 1 {
			return nil, fmt.Errorf("unsupported database version %d", seed.Version)
		}
		tx, e := db.BeginTx(context.Background(), nil)
		if e != nil {
			return nil, e
		}
		defer tx.Rollback()
		changed := map[string]bool{}
		for _, m := range seed.Messages {
			if err = r.putMessage(context.Background(), tx, m, changed); err != nil {
				return nil, err
			}
		}
		for id := range changed {
			if err = rebuildThread(context.Background(), tx, id); err != nil {
				return nil, err
			}
		}
		seed.Messages = []domain.Message{}
		seed.MailConnection = nil
		seed.MailStats = nil
		data, e := json.Marshal(seed)
		if e != nil {
			return nil, e
		}
		if _, err = tx.Exec("INSERT INTO core(id,data,revision) VALUES(1,?,1)", string(data)); err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
	}
	if err = r.initTaskIndex(context.Background()); err != nil {
		return nil, err
	}
	ok = true
	return r, nil
}
func (r *SQLiteRepository) Close() error { return r.db.Close() }
func (r *SQLiteRepository) ViewCore(ctx context.Context) (domain.State, error) {
	var st domain.State
	var data string
	err := r.db.QueryRowContext(ctx, "SELECT data FROM core WHERE id=1").Scan(&data)
	if err == nil {
		err = json.Unmarshal([]byte(data), &st)
	}
	st.Messages = []domain.Message{}
	if err == nil {
		err = reconcileMailNotifications(ctx, r.db, &st)
	}
	return st, err
}
func (r *SQLiteRepository) View(ctx context.Context) (domain.State, error) {
	st, err := r.ViewCore(ctx)
	if err != nil {
		return st, err
	}
	st.Messages, err = readMessages(ctx, r.db, "SELECT data FROM messages")
	return st, err
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readMessages(ctx context.Context, q queryer, query string, args ...any) ([]domain.Message, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Message{}
	for rows.Next() {
		var data string
		var m domain.Message
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(data), &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (r *SQLiteRepository) Update(ctx context.Context, fn func(*domain.State) error) error {
	return r.update(ctx, nil, true, fn)
}
func (r *SQLiteRepository) UpdateCore(ctx context.Context, fn func(*domain.State) error) error {
	return r.update(ctx, nil, false, fn)
}
func (r *SQLiteRepository) UpdateMessages(ctx context.Context, scope domain.MailScope, fn func(*domain.State) error) error {
	return r.update(ctx, &scope, false, fn)
}
func (r *SQLiteRepository) update(ctx context.Context, scope *domain.MailScope, all bool, fn func(*domain.State) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var data string
	var st domain.State
	if err = tx.QueryRowContext(ctx, "SELECT data FROM core WHERE id=1").Scan(&data); err != nil {
		return err
	}
	if err = json.Unmarshal([]byte(data), &st); err != nil {
		return err
	}
	st.Messages = []domain.Message{}
	if all {
		st.Messages, err = readMessages(ctx, tx, "SELECT data FROM messages")
	} else if scope != nil {
		found := map[string]domain.Message{}
		for _, id := range scope.IDs {
			ms, e := readMessages(ctx, tx, "SELECT data FROM messages WHERE id=?", id)
			if e != nil {
				return e
			}
			for _, m := range ms {
				found[m.ID] = m
			}
		}
		for _, m := range scope.Batch {
			if m.ExternalID == "" {
				continue
			}
			ms, e := readMessages(ctx, tx, `SELECT data FROM messages WHERE account=? AND source_folder=? AND external_id=? UNION SELECT data FROM messages WHERE account=? AND source_folder=? AND content_hash=? AND uid_validity<>? AND content_hash<>''`, m.AccountID, m.SourceFolder, m.ExternalID, m.AccountID, m.SourceFolder, m.ContentHash, m.UIDValidity)
			if e != nil {
				return e
			}
			for _, m := range ms {
				found[m.ID] = m
			}
		}
		for _, m := range found {
			st.Messages = append(st.Messages, m)
		}
	}
	if err != nil {
		return err
	}
	before := map[string]string{}
	for _, m := range st.Messages {
		b, _ := json.Marshal(m)
		before[m.ID] = string(b)
	}
	if err = reconcileMailNotifications(ctx, tx, &st); err != nil {
		return err
	}
	if err = fn(&st); err != nil {
		return err
	}
	changed := map[string]bool{}
	modified := false
	for _, m := range st.Messages {
		b, e := json.Marshal(m)
		if e != nil {
			return e
		}
		if before[m.ID] == string(b) {
			continue
		}
		if err = r.putMessage(ctx, tx, m, changed); err != nil {
			return err
		}
		modified = true
	}
	for id := range changed {
		if err = rebuildThread(ctx, tx, id); err != nil {
			return err
		}
	}
	st.Messages = []domain.Message{}
	st.MailConnection = nil
	st.MailStats = nil
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	revision := 0
	if modified {
		revision = 1
	}
	if _, err = tx.ExecContext(ctx, "UPDATE core SET data=?,revision=revision+? WHERE id=1", string(b), revision); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *SQLiteRepository) MailMessage(ctx context.Context, id string, demo bool) (domain.Message, error) {
	ms, e := readMessages(ctx, r.db, "SELECT data FROM messages WHERE id=? AND demo=?", id, demo)
	if e != nil {
		return domain.Message{}, e
	}
	if len(ms) == 0 {
		return domain.Message{}, sql.ErrNoRows
	}
	return ms[0], nil
}
func (r *SQLiteRepository) OwnsAttachment(ctx context.Context, key string, demo bool) (bool, error) {
	var exists bool
	e := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM mail_files WHERE demo=? AND blob_key=?)", demo, key).Scan(&exists)
	return exists, e
}
func (r *SQLiteRepository) PendingMail(ctx context.Context, version, limit int) ([]string, error) {
	rows, e := r.db.QueryContext(ctx, "SELECT id FROM messages WHERE format_version<? AND json_extract(data,'$.rawKey') IS NOT NULL ORDER BY format_version,id LIMIT ?", version, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func summary(m domain.Message) domain.Message {
	m.HTML = ""
	m.Raw = nil
	m.RawKey = ""
	m.ContentHash = ""
	m.References = ""
	m.InReplyTo = ""
	body := []rune(strings.Join(strings.Fields(m.Body), " "))
	if len(body) > 220 {
		body = body[:220]
	}
	m.Body = string(body)
	files := make([]domain.Attachment, len(m.Attachments))
	copy(files, m.Attachments)
	for i := range files {
		files[i].Data = ""
	}
	m.Attachments = files
	return m
}

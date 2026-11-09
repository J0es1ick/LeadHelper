package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"leadhelper/internal/domain"
	"os"
	"runtime"
	"testing"
	"time"
)

// Opt-in capacity check; normal unit tests use small realistic mail fixtures.
func TestMillionMessagePagination(t *testing.T) {
	if os.Getenv("LEADHELPER_LARGE_MAIL_TEST") != "1" {
		t.Skip("set LEADHELPER_LARGE_MAIL_TEST=1 for the million-message capacity check")
	}
	r := sqlFixture(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	start := time.Now()
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `WITH RECURSIVE seq(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM seq WHERE n<1000000)
 INSERT INTO messages(id,demo,folder,account,source_folder,uid_validity,external_id,content_hash,message_id,thread_id,stamp,is_read,starred,has_files,format_version,scope,subject_key,participants,summary,data)
 SELECT printf('mail-%07d',n),0,'inbox','','',0,'','','',printf('mail-%07d',n),2000000000000000-n,1,0,0,3,'false:','subject','',json_object('id',printf('mail-%07d',n),'subject','Capacity fixture','body','Short preview','date','2026-11-07T12:00:00Z','folder','inbox','read',json('true'),'attachments',json('[]')),json_object('id',printf('mail-%07d',n),'body','Full message body') FROM seq`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO threads SELECT id,0,0,id,stamp,1,0,0,0 FROM messages; INSERT INTO thread_folders SELECT id,'inbox',0,0,stamp,0 FROM messages; INSERT INTO mail_fts(rowid,id,text) SELECT rowid,id,CASE WHEN id='mail-1000000' THEN 'uniquefinalneedle' ELSE 'ordinary capacity fixture' END FROM messages`); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	t.Logf("Built one million indexed messages in %s", time.Since(start))
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for _, position := range []int{0, 500000, 999975} {
		q := domain.MailQuery{Folder: "inbox", Limit: 25}
		if position > 0 {
			q.Cursor = cursorString(2000000000000000-int64(position), fmt.Sprintf("mail-%07d", position), "false:inbox:")
		}
		at := time.Now()
		p, e := r.MailPage(ctx, false, q)
		if e != nil {
			t.Fatal(e)
		}
		if len(p.Items) != 25 {
			t.Fatalf("position %d: %d items", position, len(p.Items))
		}
		b, _ := json.Marshal(p)
		if len(b) > 50000 {
			t.Fatal("unbounded page payload")
		}
		t.Logf("Page at position %d: %s, %d bytes", position, time.Since(at), len(b))
	}
	at := time.Now()
	p, e := r.MailPage(ctx, false, domain.MailQuery{Folder: "inbox", Search: "uniquefinalneedle", Limit: 25})
	if e != nil || len(p.Items) != 1 || p.Items[0].ID != "mail-1000000" {
		t.Fatal("indexed search missed distant message", e)
	}
	t.Logf("Search across one million: %s", time.Since(at))
	runtime.ReadMemStats(&after)
	if after.TotalAlloc-before.TotalAlloc > 32<<20 {
		t.Fatal("queries allocated mailbox-sized memory")
	}
	stats, e := r.MailStats(ctx, false)
	if e != nil || stats.Folders["inbox"] != 1000000 {
		t.Fatal("folder count incorrect", e)
	}
}

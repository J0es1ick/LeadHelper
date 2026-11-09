package application

import (
	"context"
	"errors"
	"fmt"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/domain"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type resumableMail struct {
	run func(context.Context, map[string]domain.MailCheckpoint, func(domain.MailBatch) error) error
}

func (*resumableMail) Configured() bool                           { return true }
func (*resumableMail) AccountID() string                          { return "fixture-account" }
func (*resumableMail) ValidateSend(domain.Message) error          { return errors.New("read only") }
func (*resumableMail) Send(context.Context, domain.Message) error { return errors.New("read only") }
func (*resumableMail) FetchMessages(context.Context) ([]domain.Message, error) {
	panic("incremental provider must not use bulk fetch")
}
func (p *resumableMail) Sync(ctx context.Context, c map[string]domain.MailCheckpoint, save func(domain.MailBatch) error) error {
	return p.run(ctx, c, save)
}
func batch(uid, epoch uint32, hash string) domain.MailBatch {
	return domain.MailBatch{Key: "fixture-account/INBOX", Checkpoint: domain.MailCheckpoint{AccountID: "fixture-account", Folder: "INBOX", LastUID: uid, UIDValidity: epoch}, Messages: []domain.Message{{ID: fmt.Sprintf("m-%d-%d", epoch, uid), ExternalID: fmt.Sprintf("m-%d-%d", epoch, uid), AccountID: "fixture-account", SourceFolder: "INBOX", UID: uid, UIDValidity: epoch, ContentHash: hash, Subject: "Test", Folder: "inbox", Raw: []byte("original-" + hash), Attachments: []domain.Attachment{{ID: "a", Name: "a.txt", Data: "aGVsbG8="}}}}}
}
func TestMailSyncRestartAtomicCheckpointAndDedupe(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	seed := Seed(time.Now())
	seed.Settings.Demo = false
	repo, e := storage.Open(path, seed)
	if e != nil {
		t.Fatal(e)
	}
	blobs := &storage.Blobs{Directory: filepath.Join(dir, "blobs")}
	p := &resumableMail{run: func(ctx context.Context, c map[string]domain.MailCheckpoint, save func(domain.MailBatch) error) error {
		if e := save(batch(1, 100, "first")); e != nil {
			return e
		}
		return errors.New("simulated offline")
	}}
	s := New(repo, nil, p).WithBlobs(blobs)
	log, e := s.Sync(ctx, "mail")
	if e != nil || log.Success {
		t.Fatalf("expected recorded connection failure: %v", e)
	}
	reopened, e := storage.Open(path, domain.State{})
	if e != nil {
		t.Fatal(e)
	}
	s = New(reopened, nil, p).WithBlobs(blobs)
	p.run = func(ctx context.Context, c map[string]domain.MailCheckpoint, save func(domain.MailBatch) error) error {
		if c["fixture-account/INBOX"].LastUID != 1 {
			t.Fatal("checkpoint lost across restart")
		}
		return save(batch(2, 100, "second"))
	}
	if log, e = s.Sync(ctx, "mail"); e != nil || !log.Success || log.Count != 1 {
		t.Fatal("resume failed", e, log)
	}
	st, _ := s.State(ctx)
	if len(st.Messages) != 2 || st.MailCheckpoints != nil || st.MailSync.State != "idle" {
		t.Fatal("public state invalid")
	}
	first := st.Messages[0]
	raw, e := s.RawMessage(ctx, first)
	if e != nil || string(raw) != "original-first" {
		t.Fatal("original missing", e)
	}
	a := first.Attachments[0]
	if a.Data != "" || a.BlobKey == "" || a.Size != 5 {
		t.Fatal("attachment not externalized")
	}
	data, e := s.AttachmentBytes(ctx, a)
	if e != nil || string(data) != "hello" {
		t.Fatal("attachment lost")
	}
	yes := true
	s.UpdateMessage(ctx, first.ID, "archive", &yes, nil)
	// New UID epoch: same content, new external ID, preserved local state and ID.
	p.run = func(ctx context.Context, c map[string]domain.MailCheckpoint, save func(domain.MailBatch) error) error {
		return save(batch(8, 101, "first"))
	}
	log, e = s.Sync(ctx, "mail")
	if e != nil || log.Count != 0 {
		t.Fatal("epoch dedupe failed", e)
	}
	st, _ = s.State(ctx)
	if len(st.Messages) != 2 || st.Messages[0].ID != first.ID || st.Messages[0].Folder != "archive" || !st.Messages[0].Read {
		t.Fatal("local state lost after epoch reset")
	}
	draft, e := s.SaveDraft(ctx, "", domain.Message{Subject: "Forward", Attachments: []domain.Attachment{a}})
	if e != nil || draft.Attachments[0].Data != "aGVsbG8=" {
		t.Fatal("forward attachment failed", e)
	}
	a.BlobKey = "unknown"
	if _, e = s.SaveDraft(ctx, "", domain.Message{Attachments: []domain.Attachment{a}}); e == nil {
		t.Fatal("arbitrary blob accepted")
	}
}
func TestMailWorkerStartsImmediatelyAndStops(t *testing.T) {
	s, repo := fixture(t)
	repo.Update(context.Background(), func(st *domain.State) error { st.Settings.Demo = false; return nil })
	started := make(chan struct{}, 1)
	var calls atomic.Int32
	s.mail = &resumableMail{run: func(ctx context.Context, _ map[string]domain.MailCheckpoint, _ func(domain.MailBatch) error) error {
		calls.Add(1)
		started <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.RunMailWorker(ctx, time.Hour); close(done) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no immediate startup sync")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker ignored shutdown")
	}
	if calls.Load() != 1 {
		t.Fatal("unexpected overlap")
	}
	st, _ := repo.View(context.Background())
	if st.MailSync.State != "error" {
		t.Fatal("interrupted run status not persisted")
	}
}
func TestMailDisableStopsBeforeCheckpoint(t *testing.T) {
	s, repo := fixture(t)
	repo.Update(context.Background(), func(st *domain.State) error { st.Settings.Demo = false; return nil })
	s.mail = &resumableMail{run: func(ctx context.Context, _ map[string]domain.MailCheckpoint, save func(domain.MailBatch) error) error {
		repo.Update(ctx, func(st *domain.State) error { st.Settings.Modules["mail"] = false; return nil })
		return save(batch(1, 1, "blocked"))
	}}
	log, e := s.Sync(context.Background(), "mail")
	if e != nil || log.Success {
		t.Fatal("disabled module imported")
	}
	st, _ := repo.View(context.Background())
	if len(st.MailCheckpoints) != 0 {
		t.Fatal("disabled module advanced cursor")
	}
}

func TestMailSyncWaitingRequestCanBeCancelled(t *testing.T) {
	s, repo := fixture(t)
	repo.Update(context.Background(), func(st *domain.State) error { st.Settings.Demo = false; return nil })
	started := make(chan struct{})
	var calls atomic.Int32
	s.mail = &resumableMail{run: func(ctx context.Context, _ map[string]domain.MailCheckpoint, _ func(domain.MailBatch) error) error {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-ctx.Done()
		return ctx.Err()
	}}
	active, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.Sync(active, "mail") }()
	t.Cleanup(func() { stop(); <-done })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first sync did not start")
	}

	waiting, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := s.Sync(waiting, "mail"); result <- err }()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected waiting request deadline: %v", err)
		}
	case <-time.After(time.Second):
		stop()
		<-result
		t.Fatal("cancelled request remained blocked by the active sync")
	}
	st, _ := repo.View(context.Background())
	if calls.Load() != 1 || st.MailSync.State != "syncing" || len(st.SyncLogs) != 0 {
		t.Fatal("cancelled waiter changed the active synchronization")
	}
}

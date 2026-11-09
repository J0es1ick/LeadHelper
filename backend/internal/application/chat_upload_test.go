package application

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"sync/atomic"
	"testing"
)

type uploadFixture struct {
	workspaceFixture
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
	fail    bool
}

func (f *uploadFixture) SendChatFile(ctx context.Context, _ string, _ domain.ChatUpload) (domain.ChatUploadResult, error) {
	f.calls.Add(1)
	if f.started != nil {
		close(f.started)
		select {
		case <-f.release:
		case <-ctx.Done():
			return domain.ChatUploadResult{}, ctx.Err()
		}
	}
	if f.fail {
		return domain.ChatUploadResult{}, errors.New("response lost")
	}
	return domain.ChatUploadResult{MessageID: "100", FileID: "200"}, nil
}
func TestChatUploadsDeduplicateAndPreserveUncertainOutcomes(t *testing.T) {
	s, repo := fixture(t)
	ctx := context.Background()
	_ = repo.Update(ctx, func(st *domain.State) error { st.Settings.Demo = false; return nil })
	f := &uploadFixture{started: make(chan struct{}), release: make(chan struct{})}
	s.bitrix = f
	in := domain.ChatUpload{Name: "file.txt", Data: []byte("hello"), Text: "caption", AccountID: "bitrix-test", RequestID: "upload-request-0001"}
	done := make(chan error, 1)
	go func() { _, e := s.SendChatFile(ctx, "chat42", in); done <- e }()
	<-f.started
	if _, e := s.SendChatFile(ctx, "chat42", in); e == nil {
		t.Fatal("concurrent duplicate accepted")
	}
	close(f.release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if out, e := s.SendChatFile(ctx, "chat42", in); e != nil || out.MessageID != "100" || f.calls.Load() != 1 {
		t.Fatal(out, e, f.calls.Load())
	}
	in.Text = "changed"
	if _, e := s.SendChatFile(ctx, "chat42", in); e == nil || f.calls.Load() != 1 {
		t.Fatal("id reused for different caption")
	}
	f.started = nil
	f.fail = true
	in.RequestID = "upload-request-0002"
	for i := 0; i < 2; i++ {
		_, e := s.SendChatFile(ctx, "chat42", in)
		var unconfirmed ChatUploadUnconfirmed
		if !errors.As(e, &unconfirmed) {
			t.Fatal(e)
		}
	}
	if f.calls.Load() != 2 {
		t.Fatal("uncertain upload retried", f.calls.Load())
	}
	in.RequestID = "upload-request-0003"
	in.AccountID = "other"
	if _, e := s.SendChatFile(ctx, "chat42", in); e == nil || f.calls.Load() != 2 {
		t.Fatal("wrong account sent")
	}
	in.AccountID = "bitrix-test"
	in.Data = nil
	if _, e := s.SendChatFile(ctx, "chat42", in); e == nil || f.calls.Load() != 2 {
		t.Fatal("empty file sent")
	}
}

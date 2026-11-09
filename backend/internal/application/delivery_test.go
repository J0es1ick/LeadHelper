package application

import (
	"context"
	"errors"
	"leadhelper/internal/domain"
	"testing"
)

type failingMail struct {
	mockMail
	preflight bool
}

func (m *failingMail) ValidateSend(domain.Message) error {
	if m.preflight {
		return errors.New("sending disabled")
	}
	return nil
}
func (m *failingMail) Send(context.Context, domain.Message) error {
	m.sent++
	return errors.New("network result unknown")
}
func TestMailPreflightDoesNotLockDraft(t *testing.T) {
	_, repo := fixture(t)
	p := &failingMail{preflight: true}
	s := New(repo, nil, p)
	ctx := context.Background()
	st, _ := s.State(ctx)
	cfg := st.Settings
	cfg.Demo = false
	_ = s.Settings(ctx, cfg)
	m, e := s.SaveDraft(ctx, "", domain.Message{To: "test@example.test", Subject: "Test", Body: "Body"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Send(ctx, m.ID); e == nil {
		t.Fatal("preflight ignored")
	}
	st, _ = s.State(ctx)
	if len(st.Messages) != 1 || st.Messages[0].Delivery != "" || p.sent != 0 {
		t.Fatal("preflight changed delivery")
	}
	if _, e = s.SaveDraft(ctx, m.ID, m); e != nil {
		t.Fatal("preflight blocked editing")
	}
}
func TestUncertainMailCannotBeResentByEditingDraft(t *testing.T) {
	_, repo := fixture(t)
	p := &failingMail{}
	s := New(repo, nil, p)
	ctx := context.Background()
	st, _ := s.State(ctx)
	cfg := st.Settings
	cfg.Demo = false
	_ = s.Settings(ctx, cfg)
	m, e := s.SaveDraft(ctx, "", domain.Message{To: "test@example.test", Subject: "Test", Body: "Body"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Send(ctx, m.ID); e == nil {
		t.Fatal("provider failure ignored")
	}
	if _, e = s.SaveDraft(ctx, m.ID, m); e == nil {
		t.Fatal("uncertain draft editable")
	}
	if _, e = s.Send(ctx, m.ID); e == nil {
		t.Fatal("uncertain draft resent")
	}
	if p.sent != 1 {
		t.Fatal("provider called repeatedly")
	}
}

package integrations

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"leadhelper/internal/adapters/mailformat"
	"leadhelper/internal/domain"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// DeliveryError distinguishes a rejected envelope from an ambiguous DATA result.
// Neither credentials nor arbitrary server replies are exposed to the UI/logs.
type DeliveryError struct {
	Stage     string
	Uncertain bool
}

func (e *DeliveryError) Error() string           { return "SMTP: " + e.Stage }
func (e *DeliveryError) DeliveryUncertain() bool { return e.Uncertain }

type SMTP struct {
	Host, User, Password, Security string
	Port                           int
	TLSConfig                      *tls.Config // Optional trusted roots for private deployments and tests.
}

func (s *SMTP) Configured() bool { return s != nil && s.Host != "" && s.User != "" && s.Password != "" }
func (s *SMTP) ValidateSend(m domain.Message) error {
	if !s.Configured() {
		return errors.New("SMTP не подключён")
	}
	if s.Security != "tls" && s.Security != "starttls" {
		return errors.New("SMTP: требуется TLS или STARTTLS")
	}
	if s.Port < 1 || s.Port > 65535 || strings.ContainsAny(s.Host, "/\r\n@") {
		return errors.New("SMTP: проверьте сервер и порт")
	}
	from, err := mail.ParseAddress(m.From)
	if err != nil || !strings.EqualFold(from.Address, s.User) {
		return errors.New("SMTP: отправитель должен совпадать с подключённым ящиком")
	}
	return nil
}
func (s *SMTP) Send(ctx context.Context, m domain.Message) error {
	if err := s.ValidateSend(m); err != nil {
		return &DeliveryError{Stage: err.Error()}
	}
	m.Date = time.Now().UTC()
	raw, err := mailformat.Encode(m)
	if err != nil {
		return &DeliveryError{Stage: "не удалось сформировать письмо"}
	}
	return s.deliver(ctx, m, raw)
}
func (s *SMTP) deliver(ctx context.Context, m domain.Message, raw []byte) error {
	fail := func(stage string) error { return &DeliveryError{Stage: stage} }
	recipients, err := mail.ParseAddressList(m.To)
	if err != nil {
		return fail("проверьте адрес получателя")
	}
	if m.CC != "" {
		cc, e := mail.ParseAddressList(m.CC)
		if e != nil {
			return fail("проверьте адрес копии")
		}
		recipients = append(recipients, cc...)
	}
	cfg := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
	if s.TLSConfig != nil {
		cfg = s.TLSConfig.Clone()
		cfg.ServerName = s.Host
		cfg.MinVersion = tls.VersionTLS12
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	if s.Security == "tls" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: cfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fail("соединение не установлено; проверьте сеть, сервер и сертификат")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(60 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fail("сервер не ответил")
	}
	defer c.Close()
	if s.Security == "starttls" {
		if err = c.StartTLS(cfg); err != nil {
			return fail("сервер не установил защищённое соединение STARTTLS")
		}
	}
	if err = c.Auth(smtp.PlainAuth("", s.User, s.Password, s.Host)); err != nil {
		return fail("авторизация отклонена; проверьте пароль и разрешение SMTP")
	}
	if err = c.Mail(s.User); err != nil {
		return fail("сервер отклонил отправителя")
	}
	seen := map[string]bool{}
	for _, r := range recipients {
		if seen[strings.ToLower(r.Address)] {
			continue
		}
		seen[strings.ToLower(r.Address)] = true
		if err = c.Rcpt(r.Address); err != nil {
			return fail("сервер отклонил одного из получателей; письмо не отправлено")
		}
	}
	w, err := c.Data()
	if err != nil {
		return fail("сервер отказался принимать письмо")
	}
	if _, err = w.Write(raw); err != nil {
		return &DeliveryError{Stage: "соединение прервалось при передаче письма", Uncertain: true}
	}
	if err = w.Close(); err != nil {
		var rejection *textproto.Error
		if errors.As(err, &rejection) {
			return fail(fmt.Sprintf("сервер отклонил письмо (код %d)", rejection.Code))
		}
		return &DeliveryError{Stage: "нет подтверждения приёма письма", Uncertain: true}
	}
	// A successful DATA reply means accepted. QUIT failure must never invite a resend.
	_ = c.Quit()
	return nil
}

package integrations

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"leadhelper/internal/adapters/mailformat"
	"leadhelper/internal/domain"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"
)

func smtpFixture(t *testing.T, mode string) (*SMTP, <-chan []byte) {
	t.Helper()
	certServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cert := certServer.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(certServer.Certificate())
	certServer.Close()
	listener, e := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { listener.Close() })
	delivered := make(chan []byte, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		r := bufio.NewReader(conn)
		reply := func(s string) { fmt.Fprint(conn, s+"\r\n") }
		reply("220 fixture SMTP")
		for {
			line, e := r.ReadString('\n')
			if e != nil {
				return
			}
			command := strings.ToUpper(strings.Split(line, " ")[0])
			switch strings.TrimSpace(command) {
			case "EHLO":
				reply("250-fixture\r\n250-AUTH PLAIN\r\n250 8BITMIME")
			case "AUTH":
				reply("235 authenticated")
			case "MAIL":
				reply("250 sender ok")
			case "RCPT":
				if mode == "reject-recipient" {
					reply("550 private server detail")
				} else {
					reply("250 recipient ok")
				}
			case "DATA":
				reply("354 enter message")
				data, e := textproto.NewReader(r).ReadDotBytes()
				if e != nil {
					return
				}
				delivered <- data
				if mode == "uncertain" {
					return
				}
				if mode == "reject-data" {
					reply("550 rejected")
				} else {
					reply("250 accepted")
				}
			case "QUIT":
				return // lost QUIT response after DATA acceptance is still success
			default:
				reply("500 unknown")
			}
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	n, _ := strconv.Atoi(port)
	return &SMTP{Host: host, Port: n, User: "me@example.test", Password: "fixture-secret", Security: "tls", TLSConfig: &tls.Config{RootCAs: roots}}, delivered
}
func outgoingFixture() domain.Message {
	return domain.Message{From: "me@example.test", To: "you@example.test", CC: "copy@example.test", Subject: "Проверка", Body: "Текст", HTML: `<p><b>Текст</b><img src="cid:photo"></p>`, MessageID: "<unique@example.test>", InReplyTo: "<previous@example.test>", Attachments: []domain.Attachment{{ID: "photo", Name: "photo.png", Inline: true, ContentID: "photo", ContentType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("image"))}}}
}
func TestSMTPAcceptedWithLostQuitAndRichMIME(t *testing.T) {
	s, received := smtpFixture(t, "accept")
	m := outgoingFixture()
	if e := s.Send(context.Background(), m); e != nil {
		t.Fatal(e)
	}
	decoded, e := mailformat.Decode(<-received)
	if e != nil || decoded.Subject != m.Subject || decoded.MessageID != m.MessageID || decoded.InReplyTo != m.InReplyTo || len(decoded.Attachments) != 1 || decoded.Attachments[0].ContentID != "photo" || !strings.Contains(decoded.HTML, "<b>") {
		t.Fatalf("MIME roundtrip failed: %v", e)
	}
}
func TestSMTPDeliveryOutcomes(t *testing.T) {
	for _, mode := range []string{"reject-recipient", "reject-data", "uncertain"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := smtpFixture(t, mode)
			e := s.Send(context.Background(), outgoingFixture())
			var delivery *DeliveryError
			if !errors.As(e, &delivery) || delivery.Uncertain != (mode == "uncertain") {
				t.Fatalf("wrong delivery result: %v", e)
			}
			if strings.Contains(e.Error(), "fixture-secret") || strings.Contains(e.Error(), "private server detail") {
				t.Fatal("server details leaked")
			}
		})
	}
}
func TestSMTPRequiresTrustedTLSAndCorrectFrom(t *testing.T) {
	s, _ := smtpFixture(t, "accept")
	s.TLSConfig = nil
	if e := s.Send(context.Background(), outgoingFixture()); e == nil {
		t.Fatal("untrusted certificate accepted")
	}
	m := outgoingFixture()
	m.From = "other@example.test"
	if s.ValidateSend(m) == nil {
		t.Fatal("wrong sender accepted")
	}
	s.Security = "plain"
	if s.ValidateSend(outgoingFixture()) == nil {
		t.Fatal("plaintext SMTP accepted")
	}
}
func TestIMAPSentCopyUsesLiteralAndStableMessageID(t *testing.T) {
	received := make(chan []byte, 1)
	i := &IMAP{Host: "imap.example.test", User: "me@example.test", Password: "secret"}
	i.Dial = func(ctx context.Context, _ string, _ int) (*imapWire, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			r := bufio.NewReader(server)
			for step := 0; step < 3; step++ {
				line, e := r.ReadString('\n')
				if e != nil {
					return
				}
				tag, command, _ := strings.Cut(strings.TrimSpace(line), " ")
				switch step {
				case 0:
					fmt.Fprintf(server, "%s OK login\r\n", tag)
				case 1:
					io.WriteString(server, "* LIST (\\Sent) \"/\" \"Sent\"\r\n")
					fmt.Fprintf(server, "%s OK listed\r\n", tag)
				case 2:
					n := 0
					_, e = fmt.Sscanf(command, "APPEND \"Sent\" (\\Seen) {%d}", &n)
					if e != nil || n <= 0 {
						t.Error("APPEND literal missing")
						return
					}
					io.WriteString(server, "+ ready\r\n")
					raw := make([]byte, n)
					io.ReadFull(r, raw)
					r.ReadString('\n')
					received <- raw
					fmt.Fprintf(server, "%s OK appended\r\n", tag)
				}
			}
		}()
		return &imapWire{conn: client, r: bufio.NewReader(client), maxLiteral: 8 << 20}, nil
	}
	m := outgoingFixture()
	m.Date = time.Now()
	if e := i.SaveSent(context.Background(), m); e != nil {
		t.Fatal(e)
	}
	decoded, e := mailformat.Decode(<-received)
	if e != nil || decoded.MessageID != m.MessageID {
		t.Fatal("sent copy changed identity", e)
	}
}

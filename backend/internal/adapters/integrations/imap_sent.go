package integrations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"leadhelper/internal/adapters/mailformat"
	"leadhelper/internal/domain"
	"strings"
	"time"
)

// SaveSent runs only after SMTP acceptance. Failure does not change delivery status.
func (i *IMAP) SaveSent(ctx context.Context, m domain.Message) error {
	raw, err := mailformat.Encode(m)
	if err != nil {
		return err
	}
	dial := i.Dial
	if dial == nil {
		dial = openIMAP
	}
	w, err := dial(ctx, i.Host, i.port())
	if err != nil {
		return err
	}
	defer w.close()
	user, err := imapQuote(i.User)
	if err != nil {
		return err
	}
	password, err := imapQuote(i.Password)
	if err != nil {
		return err
	}
	if _, err = w.command(ctx, "LOGIN "+user+" "+password); err != nil {
		return err
	}
	rows, err := w.command(ctx, `LIST "" "*"`)
	if err != nil {
		return err
	}
	sent := ""
	for _, r := range rows {
		if len(r) >= 5 && r[1].atom("LIST") && mailboxRole(r[4].text, strings.ToLower(flatten(r[2].list))) == "sent" {
			sent = r[4].text
			break
		}
	}
	if sent == "" {
		return errors.New("IMAP: папка отправленных не найдена")
	}
	folder, err := imapQuote(sent)
	if err != nil {
		return err
	}
	w.seq++
	tag := fmt.Sprintf("L%06d", w.seq)
	deadline := time.Now().Add(20 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = w.conn.SetDeadline(deadline)
	if _, err = fmt.Fprintf(w.conn, "%s APPEND %s (\\Seen) {%d}\r\n", tag, folder, len(raw)); err != nil {
		return err
	}
	for n := 0; n < 100; n++ {
		row, e := w.response()
		if e != nil {
			return e
		}
		if len(row) > 0 && row[0].atom("+") {
			break
		}
		if len(row) > 0 && row[0].text == tag {
			return &imapCommandError{}
		}
		if n == 99 {
			return errors.New("IMAP: нет разрешения APPEND")
		}
	}
	if _, err = w.conn.Write(raw); err != nil {
		return err
	}
	if _, err = io.WriteString(w.conn, "\r\n"); err != nil {
		return err
	}
	for n := 0; n < 100; n++ {
		row, e := w.response()
		if e != nil {
			return e
		}
		if len(row) >= 2 && row[0].text == tag {
			if row[1].atom("OK") {
				return nil
			}
			return &imapCommandError{}
		}
	}
	return errors.New("IMAP: нет подтверждения APPEND")
}

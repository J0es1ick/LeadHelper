package integrations

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// The IMAP reader understands quoted strings, lists, and counted literals.
// It never logs wire commands: LOGIN contains credentials.
type imapValue struct {
	text    string
	list    []imapValue
	literal bool
}

func (v imapValue) atom(s string) bool { return strings.EqualFold(v.text, s) }

// A tagged NO/BAD completes a command without breaking the connection.
// Keep server text and the command (which can include credentials) out of errors.
type imapCommandError struct{}

func (*imapCommandError) Error() string {
	return "IMAP: сервер отклонил команду; проверьте права и параметры ящика"
}

type imapWire struct {
	conn       net.Conn
	r          *bufio.Reader
	seq        int
	maxLiteral int
	cancel     func() bool
}

func imapQuote(s string) (string, error) {
	if strings.ContainsAny(s, "\r\n\x00") {
		return "", errors.New("IMAP: недопустимые символы в настройках")
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`, nil
}
func openIMAP(ctx context.Context, host string, port int) (*imapWire, error) {
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 15 * time.Second}, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
	conn, e := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if e != nil {
		return nil, errors.New("IMAP: соединение TLS не установлено; проверьте сеть, адрес, порт и сертификат")
	}
	w := &imapWire{conn: conn, r: bufio.NewReaderSize(conn, 32768), maxLiteral: 32 << 20}
	w.cancel = context.AfterFunc(ctx, func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(45 * time.Second))
	v, e := w.response()
	if e != nil || len(v) < 2 || !v[0].atom("*") || !v[1].atom("OK") {
		w.close()
		return nil, errors.New("IMAP: сервер не прислал приветствие")
	}
	return w, nil
}
func (w *imapWire) close() {
	if w.cancel != nil {
		w.cancel()
	}
	_ = w.conn.Close()
}
func (w *imapWire) command(ctx context.Context, command string) ([][]imapValue, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	w.seq++
	tag := fmt.Sprintf("L%06d", w.seq)
	deadline := time.Now().Add(60 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = w.conn.SetDeadline(deadline)
	if _, e := io.WriteString(w.conn, tag+" "+command+"\r\n"); e != nil {
		return nil, errors.New("IMAP: не удалось отправить команду")
	}
	rows := [][]imapValue{}
	bytesRead := 0
	for {
		row, e := w.response()
		if e != nil {
			return nil, errors.New("IMAP: соединение прервалось или получен некорректный ответ")
		}
		if len(row) == 0 {
			continue
		}
		bytesRead += valuesSize(row)
		if bytesRead > 64<<20 || len(rows) > 100000 {
			return nil, errors.New("IMAP: превышен безопасный размер ответа")
		}
		if row[0].text == tag {
			if len(row) >= 2 && (row[1].atom("NO") || row[1].atom("BAD")) {
				return nil, &imapCommandError{}
			}
			if len(row) < 2 || !row[1].atom("OK") {
				return nil, errors.New("IMAP: некорректное завершение команды")
			}
			return rows, nil
		}
		if row[0].atom("*") && len(row) > 1 && row[1].atom("BYE") {
			return nil, errors.New("IMAP: сервер закрыл соединение")
		}
		rows = append(rows, row)
	}
}
func (w *imapWire) response() ([]imapValue, error) { return w.values(false, 0) }
func (w *imapWire) values(nested bool, depth int) ([]imapValue, error) {
	if depth > 30 {
		return nil, errors.New("IMAP nesting limit")
	}
	out := []imapValue{}
	for {
		b, e := w.r.ReadByte()
		if e != nil {
			return nil, e
		}
		switch b {
		case ' ', '\t':
			continue
		case '\r':
			n, e := w.r.ReadByte()
			if e != nil || n != '\n' {
				return nil, errors.New("invalid CRLF")
			}
			if nested {
				return nil, errors.New("unterminated list")
			}
			return out, nil
		case '\n':
			return nil, errors.New("bare LF")
		case ')':
			if !nested {
				return nil, errors.New("unexpected list close")
			}
			return out, nil
		case '(':
			list, e := w.values(true, depth+1)
			if e != nil {
				return nil, e
			}
			out = append(out, imapValue{list: list})
		case '"':
			var s strings.Builder
			for {
				c, e := w.r.ReadByte()
				if e != nil {
					return nil, e
				}
				if c == '"' {
					break
				}
				if c == '\\' {
					c, e = w.r.ReadByte()
					if e != nil {
						return nil, e
					}
				}
				if c == '\r' || c == '\n' || s.Len() > 65536 {
					return nil, errors.New("invalid quoted string")
				}
				s.WriteByte(c)
			}
			out = append(out, imapValue{text: s.String()})
		case '{':
			var marker strings.Builder
			for marker.Len() < 31 {
				c, e := w.r.ReadByte()
				if e != nil {
					return nil, e
				}
				marker.WriteByte(c)
				if c == '\n' {
					break
				}
			}
			line := marker.String()
			if len(line) > 30 || !strings.HasSuffix(line, "}\r\n") {
				return nil, errors.New("invalid literal")
			}
			n, e := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(line, "}\r\n"), "+"))
			if e != nil || n < 0 || n > w.maxLiteral {
				return nil, errors.New("literal too large")
			}
			data := make([]byte, n)
			if _, e = io.ReadFull(w.r, data); e != nil {
				return nil, e
			}
			out = append(out, imapValue{text: string(data), literal: true})
		default:
			var s strings.Builder
			s.WriteByte(b)
			for {
				p, e := w.r.Peek(1)
				if e != nil {
					return nil, e
				}
				if strings.ContainsRune(" \t\r\n()", rune(p[0])) {
					break
				}
				c, _ := w.r.ReadByte()
				s.WriteByte(c)
				if s.Len() > 1<<20 {
					return nil, errors.New("atom too large")
				}
			}
			out = append(out, imapValue{text: s.String()})
		}
		if len(out) > 1000000 {
			return nil, errors.New("too many tokens")
		}
	}
}
func valuesSize(values []imapValue) int {
	n := 0
	for _, v := range values {
		n += len(v.text) + valuesSize(v.list)
	}
	return n
}
func flatten(vs []imapValue) string {
	parts := []string{}
	for _, v := range vs {
		if v.list != nil {
			parts = append(parts, flatten(v.list))
		} else if !v.literal {
			parts = append(parts, v.text)
		}
	}
	return strings.Join(parts, " ")
}

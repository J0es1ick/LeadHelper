package mailformat

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"golang.org/x/net/html"
	"golang.org/x/text/encoding/htmlindex"
	"io"
	"leadhelper/internal/domain"
	"leadhelper/internal/mailbody"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

func header(s string) string { return strings.NewReplacer("\r", "", "\n", "").Replace(s) }
func Encode(m domain.Message) ([]byte, error) {
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	encodeAddresses := func(value string) string {
		addresses, e := mail.ParseAddressList(value)
		if e != nil {
			return header(value)
		}
		out := []string{}
		for _, a := range addresses {
			out = append(out, a.String())
		}
		return strings.Join(out, ", ")
	}
	from := m.From
	if a, e := mail.ParseAddress(m.From); e == nil {
		if m.FromName != "" {
			a.Name = header(m.FromName)
		}
		from = a.String()
	}
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nCc: %s\r\nSubject: %s\r\nDate: %s\r\n", header(from), encodeAddresses(m.To), encodeAddresses(m.CC), mime.QEncoding.Encode("utf-8", header(m.Subject)), m.Date.Format(time.RFC1123Z))
	for _, h := range []struct{ name, value string }{{"Message-ID", m.MessageID}, {"In-Reply-To", m.InReplyTo}, {"References", m.References}} {
		if h.value != "" {
			fmt.Fprintf(&b, "%s: %s\r\n", h.name, header(h.value))
		}
	}
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%q\r\n\r\n", mw.Boundary())
	textPart := func(w *multipart.Writer, kind, value string) error {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", kind+"; charset=utf-8")
		h.Set("Content-Transfer-Encoding", "quoted-printable")
		p, e := w.CreatePart(h)
		if e != nil {
			return e
		}
		q := quotedprintable.NewWriter(p)
		if _, e = q.Write([]byte(value)); e != nil {
			return e
		}
		return q.Close()
	}
	writer := mw
	if m.HTML != "" {
		h := textproto.MIMEHeader{}
		related := multipart.NewWriter(io.Discard)
		h.Set("Content-Type", mime.FormatMediaType("multipart/related", map[string]string{"boundary": related.Boundary()}))
		p, e := mw.CreatePart(h)
		if e != nil {
			return nil, e
		}
		writer = multipart.NewWriter(p)
		writer.SetBoundary(related.Boundary())
		alt := multipart.NewWriter(io.Discard)
		h.Set("Content-Type", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": alt.Boundary()}))
		p, e = writer.CreatePart(h)
		if e != nil {
			return nil, e
		}
		aw := multipart.NewWriter(p)
		aw.SetBoundary(alt.Boundary())
		if e = textPart(aw, "text/plain", m.Body); e != nil {
			return nil, e
		}
		if e = textPart(aw, "text/html", mailbody.Clean(m.HTML)); e != nil {
			return nil, e
		}
		if e = aw.Close(); e != nil {
			return nil, e
		}
	} else if e := textPart(mw, "text/plain", m.Body); e != nil {
		return nil, e
	}
	writeFile := func(w *multipart.Writer, a domain.Attachment) error {
		h := textproto.MIMEHeader{}
		kind, _, err := mime.ParseMediaType(a.ContentType)
		if err != nil {
			kind = "application/octet-stream"
		}
		h.Set("Content-Type", kind)
		disposition := "attachment"
		if a.Inline && a.ContentID != "" {
			disposition = "inline"
			h.Set("Content-ID", "<"+header(strings.Trim(a.ContentID, "<>"))+">")
		}
		h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.Name}))
		h.Set("Content-Transfer-Encoding", "base64")
		p, e := w.CreatePart(h)
		if e != nil {
			return e
		}
		for i := 0; i < len(a.Data); i += 76 {
			end := i + 76
			if end > len(a.Data) {
				end = len(a.Data)
			}
			if _, e = fmt.Fprint(p, a.Data[i:end]+"\r\n"); e != nil {
				return e
			}
		}
		return nil
	}
	for _, a := range m.Attachments {
		if a.Inline {
			if e := writeFile(writer, a); e != nil {
				return nil, e
			}
		}
	}
	if writer != mw {
		if e := writer.Close(); e != nil {
			return nil, e
		}
	}
	for _, a := range m.Attachments {
		if !a.Inline {
			if e := writeFile(mw, a); e != nil {
				return nil, e
			}
		}
	}
	if e := mw.Close(); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}
func Decode(data []byte) (domain.Message, error) {
	m := domain.Message{Folder: "inbox", Attachments: []domain.Attachment{}, Date: time.Now().UTC()}
	msg, e := mail.ReadMessage(bytes.NewReader(data))
	if e != nil {
		return m, e
	}
	decoder := &mime.WordDecoder{CharsetReader: charsetReader}
	m.Subject, e = decoder.DecodeHeader(msg.Header.Get("Subject"))
	if e != nil {
		m.Subject = msg.Header.Get("Subject")
	}
	m.MessageID = msg.Header.Get("Message-ID")
	m.InReplyTo = msg.Header.Get("In-Reply-To")
	m.References = msg.Header.Get("References")
	m.FormatVersion = 3
	m.From = msg.Header.Get("From")
	parser := mail.AddressParser{WordDecoder: decoder}
	if addr, e := parser.Parse(m.From); e == nil {
		m.From = addr.Address
		m.FromName = addr.Name
	}
	m.To = msg.Header.Get("To")
	m.CC = msg.Header.Get("Cc")
	if d, e := msg.Header.Date(); e == nil {
		m.Date = d
	}
	parts := 0
	var plainBody, htmlBody strings.Builder
	var walk func(textproto.MIMEHeader, io.Reader, int) error
	walk = func(h textproto.MIMEHeader, r io.Reader, depth int) error {
		parts++
		if depth > 10 || parts > 50 {
			return fmt.Errorf("Слишком сложная MIME-структура")
		}
		ct, params, e := mime.ParseMediaType(h.Get("Content-Type"))
		if e != nil {
			ct = "text/plain"
		}
		if strings.HasPrefix(ct, "multipart/") {
			mr := multipart.NewReader(r, params["boundary"])
			for {
				part, e := mr.NextPart()
				if e == io.EOF {
					break
				}
				if e != nil {
					return e
				}
				if e = walk(part.Header, part, depth+1); e != nil {
					return e
				}
			}
			return nil
		}
		switch strings.ToLower(h.Get("Content-Transfer-Encoding")) {
		case "base64":
			r = base64.NewDecoder(base64.StdEncoding, r)
		case "quoted-printable":
			r = quotedprintable.NewReader(r)
		}
		content, e := io.ReadAll(io.LimitReader(r, (25<<20)+1))
		if e != nil {
			return e
		}
		if len(content) > 25<<20 {
			return fmt.Errorf("MIME part exceeds 25 MiB")
		}
		disposition, disp, _ := mime.ParseMediaType(h.Get("Content-Disposition"))
		cid := strings.Trim(strings.TrimSpace(h.Get("Content-ID")), "<>")
		name := disp["filename"]
		if name == "" {
			name = params["name"]
		}
		if name != "" || (cid != "" && strings.HasPrefix(ct, "image/")) || disposition == "attachment" {
			if name == "" {
				name = fmt.Sprintf("image-%d", len(m.Attachments)+1)
			}
			if decoded, err := decoder.DecodeHeader(name); err == nil {
				name = decoded
			}
			m.Attachments = append(m.Attachments, domain.Attachment{ID: fmt.Sprintf("file-%d", len(m.Attachments)), Name: name, ContentType: ct, ContentID: cid, Inline: disposition == "inline" || cid != "", Size: int64(len(content)), Data: base64.StdEncoding.EncodeToString(content)})
		} else if ct == "text/plain" {
			content, e = decodeCharset(content, params["charset"])
			if e != nil {
				return e
			}
			plainBody.Write(content)
			plainBody.WriteByte('\n')
		} else if ct == "text/html" {
			content, e = decodeCharset(content, params["charset"])
			if e != nil {
				return e
			}
			htmlBody.Write(content)
		}
		return nil
	}
	e = walk(textproto.MIMEHeader(msg.Header), msg.Body, 0)
	m.HTML = mailbody.Clean(htmlBody.String())
	m.Body = strings.TrimSpace(plainBody.String())
	if m.Body == "" {
		m.Body = strings.TrimSpace(htmlText(htmlBody.String()))
	}
	return m, e
}

// Produce readable inert text. No email HTML, remote images or scripts run in UI.
func htmlText(source string) string {
	z := html.NewTokenizer(strings.NewReader(source))
	var b strings.Builder
	hidden := 0
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			break
		}
		token := z.Token()
		switch kind {
		case html.StartTagToken, html.SelfClosingTagToken:
			switch token.Data {
			case "script", "style", "template", "head":
				hidden++
			case "br", "p", "div", "tr", "li", "h1", "h2", "h3":
				if hidden == 0 {
					b.WriteByte('\n')
				}
			}
		case html.EndTagToken:
			switch token.Data {
			case "script", "style", "template", "head":
				if hidden > 0 {
					hidden--
				}
			case "p", "div", "tr", "li":
				if hidden == 0 {
					b.WriteByte('\n')
				}
			}
		case html.TextToken:
			if hidden == 0 {
				b.WriteString(token.Data)
			}
		}
	}
	lines := strings.Split(b.String(), "\n")
	out := []string{}
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
func charsetReader(label string, r io.Reader) (io.Reader, error) {
	if label == "" || strings.EqualFold(label, "utf-8") || strings.EqualFold(label, "us-ascii") {
		return r, nil
	}
	enc, e := htmlindex.Get(label)
	if e != nil {
		return nil, e
	}
	return enc.NewDecoder().Reader(r), nil
}
func decodeCharset(data []byte, label string) ([]byte, error) {
	r, e := charsetReader(label, bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	return io.ReadAll(r)
}

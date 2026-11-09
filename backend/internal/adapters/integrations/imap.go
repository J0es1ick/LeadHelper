package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"leadhelper/internal/adapters/mailformat"
	"leadhelper/internal/domain"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

type IMAP struct {
	Host, User, Password             string
	Port, BatchSize, MaxMessageBytes int
	Folders                          []string
	SMTP                             *SMTP
	Dial                             func(context.Context, string, int) (*imapWire, error)
}

func (i *IMAP) MailIdentity() (string, string) { return i.User, i.Host }
func (i *IMAP) Configured() bool               { return i.Host != "" && i.User != "" && i.Password != "" }
func (i *IMAP) AccountID() string {
	h := sha256.Sum256([]byte(strings.ToLower(i.Host) + ":" + strconv.Itoa(i.port()) + "\x00" + strings.ToLower(i.User)))
	return "imap-" + hex.EncodeToString(h[:12])
}
func (i *IMAP) port() int {
	if i.Port == 0 {
		return 993
	}
	return i.Port
}
func (i *IMAP) SendConfigured() bool                             { return i.SMTP.Configured() }
func (i *IMAP) ValidateSend(m domain.Message) error              { return i.SMTP.ValidateSend(m) }
func (i *IMAP) Send(ctx context.Context, m domain.Message) error { return i.SMTP.Send(ctx, m) }
func (i *IMAP) FetchMessages(ctx context.Context) ([]domain.Message, error) {
	out := []domain.Message{}
	e := i.Sync(ctx, nil, func(b domain.MailBatch) error { out = append(out, b.Messages...); return nil })
	return out, e
}

type mailbox struct{ wire, name, role string }

func (i *IMAP) Sync(ctx context.Context, cursors map[string]domain.MailCheckpoint, commit func(domain.MailBatch) error) error {
	if !i.Configured() {
		return errors.New("IMAP не настроен: нужны сервер, логин и пароль")
	}
	dial := i.Dial
	if dial == nil {
		dial = openIMAP
	}
	w, e := dial(ctx, i.Host, i.port())
	if e != nil {
		return e
	}
	defer w.close()
	user, e := imapQuote(i.User)
	if e != nil {
		return e
	}
	password, e := imapQuote(i.Password)
	if e != nil {
		return e
	}
	if _, e = w.command(ctx, "LOGIN "+user+" "+password); e != nil {
		return errors.New("IMAP: авторизация не выполнена; проверьте логин, пароль и разрешение IMAP у провайдера")
	}
	rows, e := w.command(ctx, `LIST "" "*"`)
	if e != nil {
		return e
	}
	folders := []mailbox{}
	for _, r := range rows {
		if len(r) < 5 || !r[1].atom("LIST") {
			continue
		}
		flags := strings.ToLower(flatten(r[2].list))
		if strings.Contains(flags, `\noselect`) {
			continue
		}
		name := r[4].text
		display := decodeMailbox(name)
		if len(i.Folders) > 0 {
			found := false
			for _, f := range i.Folders {
				if f == name || f == display {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		folders = append(folders, mailbox{name, display, mailboxRole(name, flags)})
	}
	if len(folders) == 0 {
		return errors.New("IMAP: не найдены доступные папки. Проверьте MAIL_FOLDERS")
	}
	// Start with INBOX; process every selectable folder, not only subscribed ones.
	sort.SliceStable(folders, func(a, b int) bool {
		return strings.EqualFold(folders[a].wire, "INBOX") && !strings.EqualFold(folders[b].wire, "INBOX")
	})
	var folderErrors []error
	for _, folder := range folders {
		if e = i.syncFolder(ctx, w, folder, cursors, commit); e != nil {
			folderErrors = append(folderErrors, fmt.Errorf("Папка %q: %w", folder.name, e))
			var rejected *imapCommandError
			if !errors.As(e, &rejected) || ctx.Err() != nil {
				// Network, protocol and storage failures must stop this connection.
				return errors.Join(folderErrors...)
			}
			// A rejected folder must not starve all later folders. Its checkpoint
			// stays at the last committed batch and will be retried next time.
		}
	}
	return errors.Join(folderErrors...)
}
func (i *IMAP) syncFolder(ctx context.Context, w *imapWire, f mailbox, cursors map[string]domain.MailCheckpoint, commit func(domain.MailBatch) error) error {
	quoted, e := imapQuote(f.wire)
	if e != nil {
		return e
	}
	rows, e := w.command(ctx, "EXAMINE "+quoted)
	if e != nil {
		return e
	}
	var validity uint32
	for _, r := range rows {
		for n, v := range r {
			if strings.EqualFold(v.text, "[UIDVALIDITY") && n+1 < len(r) {
				x, e := strconv.ParseUint(strings.TrimSuffix(r[n+1].text, "]"), 10, 32)
				if e == nil {
					validity = uint32(x)
				}
			}
		}
	}
	if validity == 0 {
		return errors.New("IMAP: сервер не сообщил UIDVALIDITY; прогресс не будет сдвинут")
	}
	key := i.AccountID() + "/" + base64.RawURLEncoding.EncodeToString([]byte(f.wire))
	checkpoint := cursors[key]
	last := checkpoint.LastUID
	if checkpoint.UIDValidity != validity {
		last = 0
	}
	if last == ^uint32(0) {
		return nil
	}
	rows, e = w.command(ctx, fmt.Sprintf("UID SEARCH UID %d:*", last+1))
	if e != nil {
		return e
	}
	uids := []uint32{}
	seen := map[uint32]bool{}
	for _, r := range rows {
		if len(r) < 2 || !r[1].atom("SEARCH") {
			continue
		}
		for _, v := range r[2:] {
			n, e := strconv.ParseUint(v.text, 10, 32)
			if e != nil {
				return errors.New("IMAP: некорректный UID")
			}
			uid := uint32(n)
			if uid > last && !seen[uid] {
				uids = append(uids, uid)
				seen[uid] = true
			}
		}
	}
	sort.Slice(uids, func(a, b int) bool { return uids[a] < uids[b] })
	checkpoint = domain.MailCheckpoint{AccountID: i.AccountID(), Folder: f.name, UIDValidity: validity, LastUID: last, SyncedAt: time.Now().UTC()}
	size := i.BatchSize
	if size <= 0 {
		size = 10
	}
	messages := []domain.Message{}
	for n, uid := range uids {
		if e := ctx.Err(); e != nil {
			return e
		}
		m, exists, e := i.fetch(ctx, w, f, validity, uid)
		if e != nil {
			return e
		}
		if exists {
			messages = append(messages, m)
		}
		checkpoint.LastUID = uid
		if len(messages) >= size || n == len(uids)-1 {
			checkpoint.SyncedAt = time.Now().UTC()
			if e = commit(domain.MailBatch{Key: key, Checkpoint: checkpoint, Messages: messages}); e != nil {
				return e
			}
			messages = []domain.Message{}
		}
	}
	if len(uids) == 0 {
		checkpoint.SyncedAt = time.Now().UTC()
		return commit(domain.MailBatch{Key: key, Checkpoint: checkpoint})
	}
	return nil
}
func (i *IMAP) fetch(ctx context.Context, w *imapWire, f mailbox, validity, uid uint32) (domain.Message, bool, error) {
	var zero domain.Message
	rows, e := w.command(ctx, fmt.Sprintf("UID FETCH %d (UID RFC822.SIZE FLAGS)", uid))
	if e != nil {
		return zero, false, e
	}
	exists := false
	size := 0
	flags := ""
	for _, r := range rows {
		if len(r) < 4 || !r[2].atom("FETCH") {
			continue
		}
		values := r[3].list
		match := false
		for n, v := range values {
			if v.atom("UID") && n+1 < len(values) {
				x, _ := strconv.ParseUint(values[n+1].text, 10, 32)
				match = uint32(x) == uid
			}
		}
		if !match {
			continue
		}
		exists = true
		for n, v := range values {
			if n+1 >= len(values) {
				continue
			}
			if v.atom("RFC822.SIZE") {
				size, _ = strconv.Atoi(values[n+1].text)
			}
			if v.atom("FLAGS") {
				flags = strings.ToLower(flatten(values[n+1].list))
			}
		}
	}
	if !exists {
		return zero, false, nil
	}
	max := i.MaxMessageBytes
	if max <= 0 {
		max = 25 << 20
	}
	section := "BODY.PEEK[]"
	warning := ""
	if size > max {
		section = "BODY.PEEK[HEADER]"
		warning = fmt.Sprintf("Письмо размером %d МБ превышает лимит загрузки %d МБ. Сохранены заголовки; тело и вложения остаются на сервере.", size>>20, max>>20)
	}
	rows, e = w.command(ctx, fmt.Sprintf("UID FETCH %d (UID %s)", uid, section))
	if e != nil {
		return zero, false, e
	}
	var raw []byte
	for _, r := range rows {
		if len(r) < 4 || !r[2].atom("FETCH") {
			continue
		}
		values := r[3].list
		match := false
		for n, v := range values {
			if v.atom("UID") && n+1 < len(values) {
				x, _ := strconv.ParseUint(values[n+1].text, 10, 32)
				match = uint32(x) == uid
			}
		}
		if match {
			for _, v := range values {
				if v.literal {
					raw = []byte(v.text)
				}
			}
		}
	}
	if len(raw) == 0 {
		return zero, false, errors.New("IMAP: письмо исчезло или тело не получено; повторим при следующей синхронизации")
	}
	m, e := mailformat.Decode(raw)
	if e != nil {
		// Keep the complete original even when MIME is malformed, so one poison
		// message cannot permanently block newer messages or another folder.
		m = domain.Message{Subject: "Письмо с ошибкой формата", Date: time.Now().UTC(), Attachments: []domain.Attachment{}}
		warning = "Не удалось разобрать MIME. Исходное письмо сохранено и доступно для скачивания в EML. " + warning
	}
	hash := sha256.Sum256(raw)
	m.ID = fmt.Sprintf("%s-%s-%d-%d", i.AccountID(), base64.RawURLEncoding.EncodeToString([]byte(f.wire)), validity, uid)
	m.ExternalID = m.ID
	m.AccountID = i.AccountID()
	m.SourceFolder = f.name
	m.UIDValidity = validity
	m.UID = uid
	m.ContentHash = hex.EncodeToString(hash[:])
	m.Raw = raw
	m.ImportWarning = warning
	m.Folder = f.role
	m.Read = strings.Contains(flags, `\seen`)
	m.Starred = strings.Contains(flags, `\flagged`)
	return m, true, nil
}
func mailboxRole(name, flags string) string {
	if strings.EqualFold(name, "INBOX") {
		return "inbox"
	}
	for _, v := range []struct{ flag, role string }{{`\sent`, "sent"}, {`\drafts`, "archive"}, {`\trash`, "trash"}, {`\junk`, "archive"}, {`\archive`, "archive"}} {
		if strings.Contains(flags, v.flag) {
			return v.role
		}
	}
	lower := strings.ToLower(name)
	if strings.Contains(lower, "sent") {
		return "sent"
	}
	if strings.Contains(lower, "trash") || strings.Contains(lower, "deleted") {
		return "trash"
	}
	return "archive"
}
func decodeMailbox(s string) string {
	var out strings.Builder
	for len(s) > 0 {
		n := strings.IndexByte(s, '&')
		if n < 0 {
			out.WriteString(s)
			break
		}
		out.WriteString(s[:n])
		s = s[n+1:]
		end := strings.IndexByte(s, '-')
		if end < 0 {
			out.WriteByte('&')
			out.WriteString(s)
			break
		}
		if end == 0 {
			out.WriteByte('&')
		} else {
			raw, e := base64.RawStdEncoding.DecodeString(strings.ReplaceAll(s[:end], ",", "/"))
			if e != nil || len(raw)%2 != 0 {
				out.WriteString("&" + s[:end] + "-")
			} else {
				units := make([]uint16, len(raw)/2)
				for k := range units {
					units[k] = binary.BigEndian.Uint16(raw[k*2:])
				}
				out.WriteString(string(utf16.Decode(units)))
			}
		}
		s = s[end+1:]
	}
	return out.String()
}

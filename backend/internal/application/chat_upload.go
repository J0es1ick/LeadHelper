package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"regexp"
	"strings"
	"time"
)

type chatUploadReceipt struct {
	hash    string
	at      time.Time
	running bool
	result  domain.ChatUploadResult
	err     error
}
type ChatUploadUnconfirmed struct{ Message string }

func (e ChatUploadUnconfirmed) Error() string { return e.Message }

var uploadRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{16,80}$`)
var uploadDialogID = regexp.MustCompile(`^(?:chat|sg)?[1-9][0-9]*$`)

func (s *Service) SendChatFile(ctx context.Context, dialog string, file domain.ChatUpload) (domain.ChatUploadResult, error) {
	if e := s.workspaceEnabled(ctx, "bitrix"); e != nil {
		return domain.ChatUploadResult{}, e
	}
	if s.bitrix == nil || !s.bitrix.Configured() {
		return domain.ChatUploadResult{}, invalid("Битрикс24 не подключён")
	}
	if file.AccountID == "" || file.AccountID != s.bitrix.AccountID() {
		return domain.ChatUploadResult{}, invalid("Подключение Битрикса изменилось. Откройте чат заново")
	}
	if !uploadRequestID.MatchString(file.RequestID) || !uploadDialogID.MatchString(dialog) {
		return domain.ChatUploadResult{}, invalid("Некорректный запрос отправки")
	}
	if strings.TrimSpace(file.Name) == "" || len(file.Name) > 500 || strings.ContainsAny(file.Name, "/\\\x00\r\n") || len(file.Data) == 0 || len(file.Data) > domain.ChatFileLimit || len(file.Text) > 10000 {
		return domain.ChatUploadResult{}, invalid("Добавьте непустой файл до 25 МБ и сообщение до 10 000 байт")
	}
	sender, ok := s.bitrix.(ports.WorkspaceFileSender)
	if !ok {
		return domain.ChatUploadResult{}, invalid("Отправка вложений недоступна для этого подключения")
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00", dialog, file.Name, file.Text)
	h.Write(file.Data)
	fingerprint := fmt.Sprintf("%x", h.Sum(nil))
	key := file.AccountID + ":" + file.RequestID
	s.chatUploadMu.Lock()
	if s.chatUploads == nil {
		s.chatUploads = map[string]*chatUploadReceipt{}
	}
	for key, receipt := range s.chatUploads {
		if !receipt.running && time.Since(receipt.at) > 30*time.Minute {
			delete(s.chatUploads, key)
		}
	}
	if receipt := s.chatUploads[key]; receipt != nil {
		cached := *receipt
		s.chatUploadMu.Unlock()
		if cached.hash != fingerprint {
			return domain.ChatUploadResult{}, invalid("Запрос отправки уже использован для другого файла")
		}
		if cached.running {
			return domain.ChatUploadResult{}, ChatUploadUnconfirmed{"Этот файл уже отправляется. Дождитесь результата в чате"}
		}
		return cached.result, cached.err
	}
	active := 0
	for _, r := range s.chatUploads {
		if r.running {
			active++
		}
	}
	if active >= 2 {
		s.chatUploadMu.Unlock()
		return domain.ChatUploadResult{}, invalid("Уже отправляются два файла. Дождитесь завершения")
	}
	if len(s.chatUploads) >= 1000 {
		s.chatUploadMu.Unlock()
		return domain.ChatUploadResult{}, invalid("Слишком много отправок. Повторите позже")
	}
	receipt := &chatUploadReceipt{hash: fingerprint, at: time.Now(), running: true, err: ChatUploadUnconfirmed{"Отправка прервалась. Проверьте чат перед повтором"}}
	s.chatUploads[key] = receipt
	s.chatUploadMu.Unlock()
	defer func() {
		s.chatUploadMu.Lock()
		receipt.running = false
		receipt.at = time.Now()
		s.chatUploadMu.Unlock()
	}()
	request, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	result, e := sender.SendChatFile(request, dialog, file)
	if e != nil {
		e = ChatUploadUnconfirmed{e.Error() + ". Проверьте чат перед повторной отправкой"}
	}
	s.chatUploadMu.Lock()
	receipt.result = result
	receipt.err = e
	s.chatUploadMu.Unlock()
	return result, e
}

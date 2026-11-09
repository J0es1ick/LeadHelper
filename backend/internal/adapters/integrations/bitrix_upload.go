package integrations

import (
	"context"
	"encoding/base64"
	"fmt"
	"leadhelper/internal/domain"
	"strings"
	"time"
)

func (b *Bitrix) SendChatFile(ctx context.Context, dialog string, file domain.ChatUpload) (domain.ChatUploadResult, error) {
	if file.AccountID != "" && file.AccountID != b.AccountID() {
		return domain.ChatUploadResult{}, fmt.Errorf("Подключение Битрикса изменилось. Откройте чат заново")
	}
	if !remoteID.MatchString(dialog) || file.Name == "" || len(file.Data) == 0 || len(file.Data) > domain.ChatFileLimit || len(file.Text) > 10000 {
		return domain.ChatUploadResult{}, fmt.Errorf("Проверьте сообщение и размер файла: до 25 МБ")
	}
	// A single v2 call uploads, attaches and publishes the file with its caption.
	// Never automatically retry: a lost response can follow a successful send.
	copy := *b
	c := client()
	if b.Client != nil {
		*c = *b.Client
	}
	c.Timeout = 80 * time.Second
	copy.Client = c
	var result struct {
		MessageID any `json:"messageId"`
		File      struct {
			ID any `json:"id"`
		} `json:"file"`
		DialogID string `json:"dialogId"`
	}
	_, e := copy.call(ctx, "im.v2.File.upload", map[string]any{"dialogId": dialog, "fields": map[string]any{"name": file.Name, "content": base64.StdEncoding.EncodeToString(file.Data), "message": file.Text}}, &result)
	if e != nil {
		return domain.ChatUploadResult{}, e
	}
	message, id := str(result.MessageID), str(result.File.ID)
	if !bitrixFileID.MatchString(message) || !bitrixFileID.MatchString(id) || result.DialogID != "" && !strings.EqualFold(result.DialogID, dialog) {
		return domain.ChatUploadResult{}, fmt.Errorf("Не удалось подтвердить отправку файла. Проверьте последние сообщения в чате")
	}
	return domain.ChatUploadResult{MessageID: message, FileID: id}, nil
}
func (p LiveBitrix) SendChatFile(ctx context.Context, dialog string, file domain.ChatUpload) (domain.ChatUploadResult, error) {
	return p.Hub.bitrix().SendChatFile(ctx, dialog, file)
}

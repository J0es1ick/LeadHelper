package integrations

import (
	"context"
	"errors"
	"leadhelper/internal/bitrixtext"
	"leadhelper/internal/domain"
	"strconv"
	"strings"
)

func recentMessageText(m map[string]any) string {
	if text := bitrixtext.Plain(strings.TrimSpace(str(m["text"]))); text != "" {
		return text
	}
	flag := func(v any) bool { return v == true || str(v) == "Y" || str(v) == "1" }
	switch {
	case flag(m["file"]):
		return "📎 Вложение"
	case integer(m["sticker"]) > 0:
		return "Стикер"
	case flag(m["attach"]):
		return "Прикреплённые материалы"
	case integer(m["id"]) > 0:
		return "Сообщение"
	default:
		return ""
	}
}

// Always bound the read operation to the message actually displayed by the UI.
func (b *Bitrix) ReadChat(ctx context.Context, id, messageID string) (domain.ChatRead, error) {
	message, e := strconv.ParseInt(messageID, 10, 64)
	if !remoteID.MatchString(id) || e != nil || message <= 0 {
		return domain.ChatRead{}, errors.New("Некорректный диалог или сообщение")
	}
	var result map[string]any
	if _, e = b.call(ctx, "im.dialog.read", map[string]any{"DIALOG_ID": id, "MESSAGE_ID": message}, &result); e != nil {
		return domain.ChatRead{}, e
	}
	last, e := strconv.ParseInt(str(result["lastId"]), 10, 64)
	if e != nil || last < message || str(result["dialogId"]) != id || result["counter"] == nil || integer(result["counter"]) < 0 {
		return domain.ChatRead{}, errors.New("Битрикс24 не подтвердил прочтение сообщения")
	}
	return domain.ChatRead{MessageID: messageID, Unread: integer(result["counter"])}, nil
}

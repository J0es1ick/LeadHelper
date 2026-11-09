package application

import (
	"context"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"strconv"
	"strings"
)

func readChatNotifications(st *domain.State, account, chat string, through int64) bool {
	changed := false
	prefix := account + ":messages:"
	for i := range st.Notifications {
		n := &st.Notifications[i]
		if n.Read || n.Provider != "bitrix" || n.Target != chat || !strings.HasPrefix(n.ID, prefix) {
			continue
		}
		id, e := strconv.ParseInt(strings.TrimPrefix(n.ID, prefix), 10, 64)
		if e == nil && id > 0 && id <= through {
			n.Read = true
			changed = true
		}
	}
	return changed
}
func chatReadThrough(item domain.RemoteItem) int64 {
	id, _ := strconv.ParseInt(item.ReadMessageID, 10, 64)
	if item.Unread == 0 {
		last, _ := strconv.ParseInt(item.MessageID, 10, 64)
		if last > id {
			id = last
		}
	}
	return id
}
func (s *Service) reconcileChatNotifications(ctx context.Context, page domain.RemotePage) error {
	if page.AccountID == "" || page.AccountID != s.bitrix.AccountID() {
		return nil
	}
	st, e := s.viewCore(ctx)
	if e != nil {
		return e
	}
	changed := false
	for _, item := range page.Items {
		changed = readChatNotifications(&st, page.AccountID, item.ID, chatReadThrough(item)) || changed
	}
	if !changed {
		return nil
	}
	return s.updateCore(ctx, func(current *domain.State) error {
		if current.Settings.Demo || page.AccountID != s.bitrix.AccountID() {
			return nil
		}
		for _, item := range page.Items {
			readChatNotifications(current, page.AccountID, item.ID, chatReadThrough(item))
		}
		return nil
	})
}
func (s *Service) ReadChat(ctx context.Context, chat, messageID, account string) (domain.ChatRead, error) {
	// Serialize with notification polling: a stale poll must not recreate the
	// notification between the provider acknowledgement and the local commit.
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	if e := s.workspaceEnabled(ctx, "bitrix"); e != nil {
		return domain.ChatRead{}, e
	}
	p, ok := s.bitrix.(ports.WorkspaceReadSource)
	if !ok || !s.bitrix.Configured() {
		return domain.ChatRead{}, invalid("Битрикс24 не подключён")
	}
	if account == "" || account != s.bitrix.AccountID() {
		return domain.ChatRead{}, invalid("Подключение изменилось. Обновите Битрикс24")
	}
	id, e := strconv.ParseInt(messageID, 10, 64)
	if e != nil || id <= 0 || len(chat) > 150 {
		return domain.ChatRead{}, invalid("Некорректное сообщение")
	}
	result, e := p.ReadChat(ctx, chat, messageID)
	if e != nil {
		return result, invalid(e.Error())
	}
	e = s.updateCore(ctx, func(st *domain.State) error {
		if st.Settings.Demo || account != s.bitrix.AccountID() {
			return invalid("Подключение изменилось")
		}
		readChatNotifications(st, account, chat, id)
		return nil
	})
	return result, e
}

package application

import (
	"context"
	"io"
	"leadhelper/internal/ports"
)

func (s *Service) BitrixFile(ctx context.Context, id, account string) (io.ReadCloser, string, string, int64, error) {
	if e := s.CheckBitrixFile(ctx, account); e != nil {
		return nil, "", "", 0, e
	}
	p := s.bitrix.(ports.WorkspaceFileSource)
	body, name, kind, size, e := p.DownloadFile(ctx, id)
	if e != nil {
		return nil, "", "", 0, invalid(e.Error())
	}
	return body, name, kind, size, nil
}

// Cache hits must still respect current module, demo mode and connection identity.
func (s *Service) CheckBitrixFile(ctx context.Context, account string) error {
	if e := s.workspaceEnabled(ctx, "bitrix"); e != nil {
		return e
	}
	_, ok := s.bitrix.(ports.WorkspaceFileSource)
	if !ok || !s.bitrix.Configured() {
		return invalid("Битрикс24 не подключён")
	}
	if account != s.bitrix.AccountID() {
		return invalid("Подключение изменилось. Обновите Битрикс24")
	}
	return nil
}

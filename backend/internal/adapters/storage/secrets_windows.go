//go:build windows

package storage

import (
	"errors"
	"golang.org/x/sys/windows"
	"unsafe"
)

func protectSecret(data []byte, decrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("Пустое хранилище подключений")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	var e error
	if decrypt {
		e = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		e = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if e != nil {
		return nil, errors.New("Не удалось открыть хранилище подключений. Запустите LeadHelper под той учётной записью Windows, которая сохранила подключения")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}
func encryptSecret(data []byte) ([]byte, error) { return protectSecret(data, false) }
func decryptSecret(data []byte) ([]byte, error) { return protectSecret(data, true) }

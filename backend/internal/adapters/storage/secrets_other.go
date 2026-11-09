//go:build !windows

package storage

import "errors"

func encryptSecret([]byte) ([]byte, error) {
	return nil, errors.New("В этой системе задайте подключения через секреты окружения сервера")
}
func decryptSecret([]byte) ([]byte, error) {
	return nil, errors.New("Хранилище Windows нельзя открыть в другой системе; используйте секреты окружения")
}

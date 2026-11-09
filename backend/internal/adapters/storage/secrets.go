package storage

import (
	"errors"
	"os"
	"path/filepath"
)

type Secrets struct{ Path string }

func (s Secrets) Load() ([]byte, error) {
	data, e := os.ReadFile(s.Path)
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return decryptSecret(data)
}
func (s Secrets) Save(data []byte) error {
	encrypted, e := encryptSecret(data)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(s.Path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(s.Path), ".credentials-*")
	if e != nil {
		return e
	}
	path := f.Name()
	defer os.Remove(path)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(encrypted)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(path, s.Path)
}

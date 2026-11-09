package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

// Content addressed immutable files keep MIME messages out of the JSON index.
type Blobs struct{ Directory string }

func (b *Blobs) Put(ctx context.Context, data []byte) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	hash := sha256.Sum256(data)
	key := hex.EncodeToString(hash[:])
	path := filepath.Join(b.Directory, key[:2], key)
	if info, e := os.Stat(path); e == nil && info.Size() == int64(len(data)) {
		return key, nil
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(filepath.Dir(path), "blob-*.tmp")
	if e != nil {
		return "", e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return "", e
	}
	return key, nil
}
func (b *Blobs) Get(ctx context.Context, key string) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	decoded, e := hex.DecodeString(key)
	if e != nil || len(decoded) != 32 {
		return nil, errors.New("invalid blob key")
	}
	return os.ReadFile(filepath.Join(b.Directory, key[:2], key))
}

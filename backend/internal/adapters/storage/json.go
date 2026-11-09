// Package storage implements durable storage for the single-user local deployment.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"leadhelper/internal/domain"
	"os"
	"path/filepath"
	"sync"
)

type JSONRepository struct {
	mu    sync.RWMutex
	path  string
	state domain.State
}

func Open(path string, seed domain.State) (*JSONRepository, error) {
	r := &JSONRepository{path: path, state: seed}
	b, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(b, &r.state); err != nil {
			return nil, fmt.Errorf("invalid database: %w", err)
		}
		if r.state.Version != 1 {
			return nil, fmt.Errorf("unsupported database version %d", r.state.Version)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else {
		if err = r.persist(seed); err != nil {
			return nil, err
		}
	}
	return r, nil
}
func clone(s domain.State) (domain.State, error) {
	var out domain.State
	b, e := json.Marshal(s)
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(b, &out)
	return out, e
}
func (r *JSONRepository) View(ctx context.Context) (domain.State, error) {
	if err := ctx.Err(); err != nil {
		return domain.State{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return clone(r.state)
}
func (r *JSONRepository) Update(ctx context.Context, fn func(*domain.State) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	next, err := clone(r.state)
	if err != nil {
		return err
	}
	if err = fn(&next); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = r.persist(next); err != nil {
		return err
	}
	r.state = next
	return nil
}
func (r *JSONRepository) persist(s domain.State) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(r.path), ".leadhelper-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, r.path)
}

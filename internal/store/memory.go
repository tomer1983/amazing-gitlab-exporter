package store

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-memory implementation of Store.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]time.Time
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]time.Time),
	}
}

func (m *MemoryStore) GetLastUpdated(_ context.Context, key string) (time.Time, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.data[key]
	if !ok {
		return time.Time{}, nil
	}
	return t, nil
}

func (m *MemoryStore) SetLastUpdated(_ context.Context, key string, t time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = t
	return nil
}

func (m *MemoryStore) Close() error {
	return nil
}

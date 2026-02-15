// Package store provides storage abstractions for the exporter.
package store

import (
	"context"
	"time"
)

// Store is the interface for persisting exporter state (last-fetched timestamps, etc.).
type Store interface {
	// GetLastUpdated returns the last-updated timestamp for a given key (project+collector).
	GetLastUpdated(ctx context.Context, key string) (time.Time, error)
	// SetLastUpdated records the last-updated timestamp for a key.
	SetLastUpdated(ctx context.Context, key string, t time.Time) error
	// Close releases any resources held by the store.
	Close() error
}

package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisKeyPrefix = "age:last_updated:"

// RedisStore implements the Store interface using Redis.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a new RedisStore connected to the given Redis URL.
// The URL is parsed with redis.ParseURL so it supports redis:// and rediss:// schemes.
func NewRedisStore(url string) (*RedisStore, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Verify connectivity.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}

	return &RedisStore{client: client}, nil
}

// GetLastUpdated returns the last-updated timestamp for the given key.
// If the key does not exist, a zero time.Time is returned.
func (r *RedisStore) GetLastUpdated(ctx context.Context, key string) (time.Time, error) {
	val, err := r.client.Get(ctx, redisKeyPrefix+key).Result()
	if err == redis.Nil {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("redis GET %s: %w", key, err)
	}

	epoch, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing stored timestamp for %s: %w", key, err)
	}

	return time.Unix(epoch, 0), nil
}

// SetLastUpdated stores the given timestamp as a Unix epoch string in Redis.
func (r *RedisStore) SetLastUpdated(ctx context.Context, key string, t time.Time) error {
	val := strconv.FormatInt(t.Unix(), 10)
	if err := r.client.Set(ctx, redisKeyPrefix+key, val, 0).Err(); err != nil {
		return fmt.Errorf("redis SET %s: %w", key, err)
	}
	return nil
}

// Close closes the Redis client connection.
func (r *RedisStore) Close() error {
	return r.client.Close()
}

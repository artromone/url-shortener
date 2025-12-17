package cache

import (
	"context"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

type Cache struct {
	rdb      *redis.Client
	localMu  sync.RWMutex
	localMap map[string]cacheEntry
}

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

func New(rdb *redis.Client) *Cache {
	c := &Cache{
		rdb:      rdb,
		localMap: make(map[string]cacheEntry),
	}

	go c.cleanup()
	return c
}

func (c *Cache) Get(ctx context.Context, key string) (string, bool, error) {
	c.localMu.RLock()
	if entry, ok := c.localMap[key]; ok {
		if time.Now().Before(entry.expiresAt) {
			c.localMu.RUnlock()
			return entry.value, true, nil
		}
	}
	c.localMu.RUnlock()

	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	c.localMu.Lock()
	c.localMap[key] = cacheEntry{
		value:     val,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	c.localMu.Unlock()

	return val, true, nil
}

func (c *Cache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		return err
	}

	c.localMu.Lock()
	c.localMap[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	c.localMu.Unlock()

	return nil
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	c.localMu.Lock()
	delete(c.localMap, key)
	c.localMu.Unlock()

	return c.rdb.Del(ctx, key).Err()
}

func (c *Cache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.localMu.Lock()
		now := time.Now()
		for key, entry := range c.localMap {
			if now.After(entry.expiresAt) {
				delete(c.localMap, key)
			}
		}
		c.localMu.Unlock()
	}
}

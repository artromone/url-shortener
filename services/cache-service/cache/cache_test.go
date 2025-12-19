package cache

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1, // Используем отдельную БД для тестов
	})

	// Очищаем тестовую БД
	err := rdb.FlushDB(context.Background()).Err()
	require.NoError(t, err)

	return rdb
}

func TestCacheSetAndGet(t *testing.T) {
	rdb := setupTestRedis(t)
	defer rdb.Close()

	c := New(rdb)
	ctx := context.Background()

	err := c.Set(ctx, "test-key", "test-value", 1*time.Minute)
	require.NoError(t, err)

	value, found, err := c.Get(ctx, "test-key")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "test-value", value)
}

func TestCacheGetNotFound(t *testing.T) {
	rdb := setupTestRedis(t)
	defer rdb.Close()

	c := New(rdb)
	ctx := context.Background()

	value, found, err := c.Get(ctx, "non-existent-key")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
}

func TestCacheDelete(t *testing.T) {
	rdb := setupTestRedis(t)
	defer rdb.Close()

	c := New(rdb)
	ctx := context.Background()

	err := c.Set(ctx, "delete-test", "value", 1*time.Minute)
	require.NoError(t, err)

	err = c.Delete(ctx, "delete-test")
	require.NoError(t, err)

	_, found, err := c.Get(ctx, "delete-test")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestCacheLocalMemory(t *testing.T) {
	rdb := setupTestRedis(t)
	defer rdb.Close()

	c := New(rdb)
	ctx := context.Background()

	// Устанавливаем в Redis
	err := c.Set(ctx, "local-test", "value", 1*time.Minute)
	require.NoError(t, err)

	// Первое чтение (из Redis)
	value1, found1, err := c.Get(ctx, "local-test")
	require.NoError(t, err)
	assert.True(t, found1)
	assert.Equal(t, "value", value1)

	// Проверяем что в локальном кэше
	c.localMu.RLock()
	_, existsLocal := c.localMap["local-test"]
	c.localMu.RUnlock()
	assert.True(t, existsLocal, "Should be in local cache")

	// Удаляем из Redis напрямую
	rdb.Del(ctx, "local-test")

	// Второе чтение (из локального кэша)
	value2, found2, err := c.Get(ctx, "local-test")
	require.NoError(t, err)
	assert.True(t, found2, "Should find in local cache")
	assert.Equal(t, "value", value2)
}

func TestCacheExpiration(t *testing.T) {
	rdb := setupTestRedis(t)
	defer rdb.Close()

	c := New(rdb)
	ctx := context.Background()

	// Устанавливаем с коротким TTL
	err := c.Set(ctx, "expire-test", "value", 100*time.Millisecond)
	require.NoError(t, err)

	// Сразу должно быть доступно
	_, found, err := c.Get(ctx, "expire-test")
	require.NoError(t, err)
	assert.True(t, found)

	// Ждем истечения
	time.Sleep(200 * time.Millisecond)

	// Должно исчезнуть из Redis
	_, found, err = c.Get(ctx, "expire-test")
	require.NoError(t, err)
	assert.False(t, found)
}

func BenchmarkCacheGet(b *testing.B) {
	rdb := setupTestRedis(&testing.T{})
	defer rdb.Close()

	c := New(rdb)
	ctx := context.Background()

	c.Set(ctx, "bench-key", "bench-value", 1*time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(ctx, "bench-key")
	}
}

func BenchmarkCacheSet(b *testing.B) {
	rdb := setupTestRedis(&testing.T{})
	defer rdb.Close()

	c := New(rdb)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(ctx, "bench-key", "bench-value", 1*time.Hour)
	}
}

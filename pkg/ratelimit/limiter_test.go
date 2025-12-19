package ratelimit

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLimiterAllow(t *testing.T) {
	limiter := New(5, time.Second)
	key := "test-key"

	// Должны пройти первые 5 запросов
	for i := 0; i < 5; i++ {
		allowed := limiter.Allow(key)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// 6-й запрос должен быть заблокирован
	allowed := limiter.Allow(key)
	assert.False(t, allowed, "Request 6 should be blocked")
}

func TestLimiterTokenRefill(t *testing.T) {
	limiter := New(2, 100*time.Millisecond)
	key := "refill-test"

	// Используем 2 токена
	assert.True(t, limiter.Allow(key))
	assert.True(t, limiter.Allow(key))
	assert.False(t, limiter.Allow(key))

	// Ждем пополнения
	time.Sleep(250 * time.Millisecond)

	// Должны появиться новые токены
	assert.True(t, limiter.Allow(key))
	assert.True(t, limiter.Allow(key))
}

func TestLimiterConcurrency(t *testing.T) {
	limiter := New(100, time.Second)
	key := "concurrent-test"

	var wg sync.WaitGroup
	allowed := make(chan bool, 200)

	// 200 конкурентных запросов
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- limiter.Allow(key)
		}()
	}

	wg.Wait()
	close(allowed)

	allowedCount := 0
	for a := range allowed {
		if a {
			allowedCount++
		}
	}

	// Должно пройти примерно 100 запросов
	assert.GreaterOrEqual(t, allowedCount, 90, "Should allow ~100 requests")
	assert.LessOrEqual(t, allowedCount, 110, "Should not allow much more than 100")
}

func TestLimiterDifferentKeys(t *testing.T) {
	limiter := New(2, time.Second)

	// Разные ключи имеют независимые лимиты
	assert.True(t, limiter.Allow("key1"))
	assert.True(t, limiter.Allow("key1"))
	assert.False(t, limiter.Allow("key1"))

	// key2 должен работать независимо
	assert.True(t, limiter.Allow("key2"))
	assert.True(t, limiter.Allow("key2"))
	assert.False(t, limiter.Allow("key2"))
}

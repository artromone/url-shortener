package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	tokens map[string]*bucket
	mu     sync.RWMutex
	rate   int
	per    time.Duration
}

type bucket struct {
	tokens   int
	lastSeen time.Time
}

func New(rate int, per time.Duration) *Limiter {
	l := &Limiter{
		tokens: make(map[string]*bucket),
		rate:   rate,
		per:    per,
	}

	go l.cleanup()
	return l
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.tokens[key]

	if !exists {
		l.tokens[key] = &bucket{
			tokens:   l.rate - 1,
			lastSeen: now,
		}
		return true
	}

	elapsed := now.Sub(b.lastSeen)
	tokensToAdd := int(elapsed / l.per)

	if tokensToAdd > 0 {
		b.tokens = min(l.rate, b.tokens+tokensToAdd)
		b.lastSeen = now
	}

	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

func (l *Limiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for key, b := range l.tokens {
			if now.Sub(b.lastSeen) > 5*time.Minute {
				delete(l.tokens, key)
			}
		}
		l.mu.Unlock()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

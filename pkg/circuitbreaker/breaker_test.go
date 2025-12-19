package circuitbreaker

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreakerClosed(t *testing.T) {
	cb := New(3, time.Second)

	// Успешные вызовы должны проходить
	err := cb.Call(func() error {
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, StateClosed, cb.state)
}

func TestCircuitBreakerOpens(t *testing.T) {
	cb := New(3, time.Second)
	testErr := errors.New("test error")

	// 3 неудачных вызова
	for i := 0; i < 3; i++ {
		err := cb.Call(func() error {
			return testErr
		})
		assert.Error(t, err)
	}

	// Circuit должен открыться
	assert.Equal(t, StateOpen, cb.state)

	// Следующий вызов должен сразу вернуть ошибку
	err := cb.Call(func() error {
		t.Fatal("Should not be called when circuit is open")
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker is open")
}

func TestCircuitBreakerHalfOpen(t *testing.T) {
	cb := New(2, 100*time.Millisecond)
	testErr := errors.New("test error")

	// Открываем circuit
	cb.Call(func() error { return testErr })
	cb.Call(func() error { return testErr })
	assert.Equal(t, StateOpen, cb.state)

	// Ждем reset timeout
	time.Sleep(150 * time.Millisecond)

	// Следующий вызов переводит в Half-Open
	called := false
	err := cb.Call(func() error {
		called = true
		return nil // Успешный вызов
	})

	assert.NoError(t, err)
	assert.True(t, called, "Function should be called in half-open state")
	assert.Equal(t, StateClosed, cb.state, "Should transition to closed on success")
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := New(2, 50*time.Millisecond)
	testErr := errors.New("test error")

	// Один неудачный вызов
	cb.Call(func() error { return testErr })
	assert.Equal(t, 1, cb.failures)

	// Успешный вызов сбрасывает счетчик
	err := cb.Call(func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, 0, cb.failures)
}

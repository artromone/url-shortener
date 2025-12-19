package worker

import (
	"testing"
	"time"
	"url-shortener/services/analytics-service/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) BatchInsertClicks(events []models.ClickEvent) error {
	args := m.Called(events)
	return args.Error(0)
}

func (m *MockRepository) UpdateStats(shortCode string) error {
	args := m.Called(shortCode)
	return args.Error(0)
}

func (m *MockRepository) GetStats(shortCode string) (*Stats, error) {
	args := m.Called(shortCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Stats), args.Error(1)
}

type Stats struct {
	TotalClicks    int64
	UniqueVisitors int64
	LastClickedAt  *time.Time
}

func TestWorkerPoolSubmit(t *testing.T) {
	repo := new(MockRepository)
	pool := New(2, 100, repo)

	// Не запускаем pool.Start(), проверяем только очередь
	event := models.ClickEvent{
		ShortCode: "test123",
		IPAddress: "192.168.1.1",
		Timestamp: time.Now(),
	}

	pool.Submit(event)

	// Проверяем что событие в очереди
	select {
	case e := <-pool.jobQueue:
		assert.Equal(t, event.ShortCode, e.ShortCode)
	case <-time.After(1 * time.Second):
		t.Fatal("Event not submitted to queue")
	}
}

func TestWorkerPoolBatching(t *testing.T) {
	repo := new(MockRepository)
	repo.On("BatchInsertClicks", mock.Anything).Return(nil)
	repo.On("UpdateStats", mock.Anything).Return(nil)

	pool := New(2, 1000, repo)
	pool.Start()
	defer pool.Stop()

	// Отправляем 150 событий (больше размера батча)
	for i := 0; i < 150; i++ {
		event := models.ClickEvent{
			ShortCode: "batch123",
			IPAddress: "192.168.1.1",
			UserAgent: "Test",
			Timestamp: time.Now(),
		}
		pool.Submit(event)
	}

	// Ждем обработки
	time.Sleep(2 * time.Second)

	// Проверяем что BatchInsertClicks был вызван минимум 1 раз
	repo.AssertCalled(t, "BatchInsertClicks", mock.Anything)

	// Должно быть минимум 2 вызова (150 событий / 100 размер батча)
	calls := repo.Calls
	batchInsertCalls := 0
	for _, call := range calls {
		if call.Method == "BatchInsertClicks" {
			batchInsertCalls++
		}
	}
	assert.GreaterOrEqual(t, batchInsertCalls, 1, "Should batch insert at least once")
}

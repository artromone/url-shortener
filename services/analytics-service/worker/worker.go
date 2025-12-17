package worker

import (
	"log"
	"sync"
	"time"

	"url-shortener/services/analytics-service/models"
	"url-shortener/services/analytics-service/repository"
)

type WorkerPool struct {
	workers    int
	jobQueue   chan models.ClickEvent
	batchQueue chan []models.ClickEvent
	repo       *repository.Repository
	wg         sync.WaitGroup
	quit       chan struct{}
}

func New(workers, queueSize int, repo *repository.Repository) *WorkerPool {
	return &WorkerPool{
		workers:    workers,
		jobQueue:   make(chan models.ClickEvent, queueSize),
		batchQueue: make(chan []models.ClickEvent, 100),
		repo:       repo,
		quit:       make(chan struct{}),
	}
}

// Вызывается из сервиса один раз после создания пула.
func (p *WorkerPool) Start() {
	// 1) Запускаем воркеры, которые читают батчи и пишут в БД.
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i + 1)
	}

	// 2) Запускаем агрегатор, который собирает события в батчи.
	p.wg.Add(1)
	go p.batchCollector()
}

func (p *WorkerPool) Submit(event models.ClickEvent) {
	select {
	case p.jobQueue <- event:
	default:
		log.Println("Queue full, dropping event")
	}
}

func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	for batch := range p.batchQueue {
		if err := p.repo.BatchInsertClicks(batch); err != nil {
			log.Printf("Worker %d failed to insert batch: %v", id, err)
		}

		for _, event := range batch {
			if err := p.repo.UpdateStats(event.ShortCode); err != nil {
				log.Printf("Worker %d failed to update stats: %v", id, err)
			}
		}
	}
}

// Собирает события из jobQueue в батчи и отправляет в batchQueue.
func (p *WorkerPool) batchCollector() {
	defer p.wg.Done()

	const (
		maxBatchSize  = 100
		flushInterval = 1 * time.Second
	)

	batch := make([]models.ClickEvent, 0, maxBatchSize)
	timer := time.NewTimer(flushInterval)
	defer timer.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Отправляем копию, чтобы не гонять один и тот же слайс.
		batchCopy := make([]models.ClickEvent, len(batch))
		copy(batchCopy, batch)
		select {
		case p.batchQueue <- batchCopy:
		default:
			log.Println("batchQueue full, dropping batch")
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-p.quit:
			// Дочищаем оставшиеся события.
			flush()
			close(p.batchQueue)
			return

		case ev := <-p.jobQueue:
			batch = append(batch, ev)
			if len(batch) >= maxBatchSize {
				flush()
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(flushInterval)
			}

		case <-timer.C:
			flush()
			timer.Reset(flushInterval)
		}
	}
}

func (p *WorkerPool) Stop() {
	close(p.quit)
	p.wg.Wait()
}


package worker

import (
	"log"
	"sync"
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

func (p *WorkerPool) Submit(event models.ClickEvent) {
	// Rest of the code remains the same
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

func (p *WorkerPool) Stop() {
	close(p.quit)
	p.wg.Wait()
}

package worker

import (
	"log"
	"sync"
	"time"

	"github.com/artromone/url-shortener/services/analytics-service/repository"
)

type ClickEvent struct {
	ShortCode string
	IPAddress string
	UserAgent string
	Referer   string
	Country   string
	Timestamp time.Time
}

type WorkerPool struct {
	workers    int
	jobQueue   chan ClickEvent
	batchQueue chan []ClickEvent
	repo       *repository.Repository
	wg         sync.WaitGroup
	quit       chan struct{}
}

func New(workers, queueSize int, repo *repository.Repository) *WorkerPool {
	return &WorkerPool{
		workers:    workers,
		jobQueue:   make(chan ClickEvent, queueSize),
		batchQueue: make(chan []ClickEvent, 100),
		repo:       repo,
		quit:       make(chan struct{}),
	}
}

func (p *WorkerPool) Start() {
	go p.batcher()

	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

func (p *WorkerPool) Submit(event ClickEvent) {
	select {
	case p.jobQueue = 100 {
				p.batchQueue  0 {
				p.batchQueue  0 {
				p.batchQueue <- batch
			}
			close(p.batchQueue)
			return
		}
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

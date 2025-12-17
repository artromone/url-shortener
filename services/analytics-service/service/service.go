package service

import (
	"context"
	"time"

	"url-shortener/services/analytics-service/repository"
	"url-shortener/services/analytics-service/worker"

	pb "url-shortener/proto/generated/analytics_service"
)

type Service struct {
	pb.UnimplementedAnalyticsServiceServer
	pool *worker.WorkerPool
	repo *repository.Repository
}

func New(pool *worker.WorkerPool, repo *repository.Repository) *Service {
	return &Service{
		pool: pool,
		repo: repo,
	}
}

func (s *Service) RecordClick(ctx context.Context, req *pb.ClickEvent) (*pb.ClickResponse, error) {
	event := worker.ClickEvent{
		ShortCode: req.ShortCode,
		IPAddress: req.IpAddress,
		UserAgent: req.UserAgent,
		Referer:   req.Referer,
		Country:   req.Country,
		Timestamp: time.Now(),
	}

	s.pool.Submit(event)

	return &pb.ClickResponse{Success: true}, nil
}

func (s *Service) GetStatistics(ctx context.Context, req *pb.StatsRequest) (*pb.StatsResponse, error) {
	stats, err := s.repo.GetStats(req.ShortCode)
	if err != nil {
		return nil, err
	}

	var lastClicked string
	if stats.LastClickedAt != nil {
		lastClicked = stats.LastClickedAt.Format(time.RFC3339)
	}

	return &pb.StatsResponse{
		TotalClicks:    stats.TotalClicks,
		UniqueVisitors: stats.UniqueVisitors,
		LastClickedAt:  lastClicked,
	}, nil
}

package service

import (
	"context"
	"database/sql"
	"time"

	"url-shortener/pkg/shortcode"
	"url-shortener/services/url-service/repository"

	pb "url-shortener/proto/generated/url_service"
)

type Service struct {
	pb.UnimplementedURLServiceServer
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateShortURL(ctx context.Context, req *pb.CreateURLRequest) (*pb.CreateURLResponse, error) {
	var expiresAt *time.Time
	if req.ExpiresInDays > 0 {
		t := time.Now().AddDate(0, 0, int(req.ExpiresInDays))
		expiresAt = &t
	}

	var code string
	var err error

	for i := 0; i < 5; i++ {
		code, err = shortcode.Generate()
		if err != nil {
			return &pb.CreateURLResponse{Success: false, Error: "failed to generate code"}, nil
		}

		exists, _ := s.repo.Exists(code)
		if !exists {
			break
		}
	}

	url := &repository.URL{
		ShortCode:   code,
		OriginalURL: req.OriginalUrl,
		UserID:      req.UserId,
		ExpiresAt:   expiresAt,
		IsActive:    true,
	}

	if err := s.repo.Create(url); err != nil {
		return &pb.CreateURLResponse{Success: false, Error: err.Error()}, nil
	}

	return &pb.CreateURLResponse{
		ShortCode: code,
		ShortUrl:  "http://localhost:8080/" + code,
		Success:   true,
	}, nil
}

func (s *Service) GetOriginalURL(ctx context.Context, req *pb.GetURLRequest) (*pb.GetURLResponse, error) {
	url, err := s.repo.GetByShortCode(req.ShortCode)
	if err != nil {
		if err == sql.ErrNoRows {
			return &pb.GetURLResponse{Found: false}, nil
		}
		return nil, err
	}

	if url.ExpiresAt != nil && time.Now().After(*url.ExpiresAt) {
		return &pb.GetURLResponse{Found: true, IsActive: false}, nil
	}

	return &pb.GetURLResponse{
		OriginalUrl: url.OriginalURL,
		Found:       true,
		IsActive:    url.IsActive,
	}, nil
}

func (s *Service) DeleteURL(ctx context.Context, req *pb.DeleteURLRequest) (*pb.DeleteURLResponse, error) {
	err := s.repo.Delete(req.ShortCode)
	return &pb.DeleteURLResponse{Success: err == nil}, err
}

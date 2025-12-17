package service

import (
	"context"
	"time"

	pb "github.com/artromone/url-shortener/proto/cache_service"
	"github.com/artromone/url-shortener/services/cache-service/cache"
)

type Service struct {
	pb.UnimplementedCacheServiceServer
	cache *cache.Cache
}

func New(cache *cache.Cache) *Service {
	return &Service{cache: cache}
}

func (s *Service) Get(ctx context.Context, req *pb.CacheGetRequest) (*pb.CacheGetResponse, error) {
	value, found, err := s.cache.Get(ctx, req.Key)
	if err != nil {
		return nil, err
	}

	return &pb.CacheGetResponse{
		Value: value,
		Found: found,
	}, nil
}

func (s *Service) Set(ctx context.Context, req *pb.CacheSetRequest) (*pb.CacheSetResponse, error) {
	ttl := time.Duration(req.TtlSeconds) * time.Second
	if ttl == 0 {
		ttl = 24 * time.Hour
	}

	err := s.cache.Set(ctx, req.Key, req.Value, ttl)
	return &pb.CacheSetResponse{Success: err == nil}, err
}

func (s *Service) Delete(ctx context.Context, req *pb.CacheDeleteRequest) (*pb.CacheDeleteResponse, error) {
	err := s.cache.Delete(ctx, req.Key)
	return &pb.CacheDeleteResponse{Success: err == nil}, err
}

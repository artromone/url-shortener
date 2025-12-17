package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"url-shortener/services/cache-service/cache"
	"url-shortener/services/cache-service/service"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "url-shortener/proto/generated/cache_service"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		PoolSize: 10,
	})

	cacheManager := cache.New(rdb)
	svc := service.New(cacheManager)

	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterCacheServiceServer(grpcServer, svc)
	reflection.Register(grpcServer)

	go func() {
		log.Println("Cache Service listening on :50053")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Cache Service...")
	grpcServer.GracefulStop()
	rdb.Close()
}

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	analyticsProto "github.com/artromone/url-shortener/proto/analytics_service"
	cacheProto "github.com/artromone/url-shortener/proto/cache_service"
	urlProto "github.com/artromone/url-shortener/proto/url_service"
	"github.com/artromone/url-shortener/services/api-gateway/handlers"
	"github.com/artromone/url-shortener/services/api-gateway/middleware"
)

func main() {
	urlConn, err := grpc.Dial(
		getEnv("URL_SERVICE_ADDR", "localhost:50051"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect to URL service: %v", err)
	}
	defer urlConn.Close()

	analyticsConn, err := grpc.Dial(
		getEnv("ANALYTICS_SERVICE_ADDR", "localhost:50052"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect to Analytics service: %v", err)
	}
	defer analyticsConn.Close()

	cacheConn, err := grpc.Dial(
		getEnv("CACHE_SERVICE_ADDR", "localhost:50053"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect to Cache service: %v", err)
	}
	defer cacheConn.Close()

	urlClient := urlProto.NewURLServiceClient(urlConn)
	analyticsClient := analyticsProto.NewAnalyticsServiceClient(analyticsConn)
	cacheClient := cacheProto.NewCacheServiceClient(cacheConn)

	h := handlers.New(urlClient, analyticsClient, cacheClient)

	r := gin.Default()

	rateLimiter := middleware.NewRateLimiter(100, time.Minute)
	r.Use(rateLimiter.Middleware())

	r.POST("/shorten", h.ShortenURL)
	r.GET("/:shortCode", h.RedirectURL)
	r.GET("/stats/:shortCode", h.GetStats)
	r.DELETE("/:shortCode", h.DeleteURL)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		log.Println("API Gateway listening on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down API Gateway...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

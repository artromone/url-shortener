package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	analyticsProto "github.com/artromone/url-shortener/proto/analytics_service"
	cacheProto "github.com/artromone/url-shortener/proto/cache_service"
	urlProto "github.com/artromone/url-shortener/proto/url_service"
)

type Handler struct {
	urlClient       urlProto.URLServiceClient
	analyticsClient analyticsProto.AnalyticsServiceClient
	cacheClient     cacheProto.CacheServiceClient
}

func New(urlClient urlProto.URLServiceClient, analyticsClient analyticsProto.AnalyticsServiceClient, cacheClient cacheProto.CacheServiceClient) *Handler {
	return &Handler{
		urlClient:       urlClient,
		analyticsClient: analyticsClient,
		cacheClient:     cacheClient,
	}
}

type ShortenRequest struct {
	URL           string `json:"url" binding:"required,url"`
	UserID        string `json:"user_id"`
	ExpiresInDays int64  `json:"expires_in_days"`
}

func (h *Handler) ShortenURL(c *gin.Context) {
	var req ShortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.urlClient.CreateShortURL(ctx, &urlProto.CreateURLRequest{
		OriginalUrl:   req.URL,
		UserId:        req.UserID,
		ExpiresInDays: req.ExpiresInDays,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !resp.Success {
		c.JSON(http.StatusInternalServerError, gin.H{"error": resp.Error})
		return
	}

	h.cacheClient.Set(ctx, resp.ShortCode, req.URL, &cacheProto.CacheSetRequest{
		Key:        resp.ShortCode,
		Value:      req.URL,
		TtlSeconds: 3600,
	})

	c.JSON(http.StatusOK, gin.H{
		"short_code": resp.ShortCode,
		"short_url":  resp.ShortUrl,
	})
}

func (h *Handler) RedirectURL(c *gin.Context) {
	shortCode := c.Param("shortCode")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cacheResp, _ := h.cacheClient.Get(ctx, &cacheProto.CacheGetRequest{Key: shortCode})
	if cacheResp != nil && cacheResp.Found {
		go h.recordClick(shortCode, c)
		c.Redirect(http.StatusMovedPermanently, cacheResp.Value)
		return
	}

	resp, err := h.urlClient.GetOriginalURL(ctx, &urlProto.GetURLRequest{ShortCode: shortCode})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !resp.Found {
		c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
		return
	}

	if !resp.IsActive {
		c.JSON(http.StatusGone, gin.H{"error": "URL has expired"})
		return
	}

	h.cacheClient.Set(ctx, &cacheProto.CacheSetRequest{
		Key:        shortCode,
		Value:      resp.OriginalUrl,
		TtlSeconds: 3600,
	})

	go h.recordClick(shortCode, c)

	c.Redirect(http.StatusMovedPermanently, resp.OriginalUrl)
}

func (h *Handler) GetStats(c *gin.Context) {
	shortCode := c.Param("shortCode")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.analyticsClient.GetStatistics(ctx, &analyticsProto.StatsRequest{
		ShortCode: shortCode,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"short_code":      shortCode,
		"total_clicks":    resp.TotalClicks,
		"unique_visitors": resp.UniqueVisitors,
		"last_clicked_at": resp.LastClickedAt,
	})
}

func (h *Handler) DeleteURL(c *gin.Context) {
	shortCode := c.Param("shortCode")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.urlClient.DeleteURL(ctx, &urlProto.DeleteURLRequest{ShortCode: shortCode})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.cacheClient.Delete(ctx, &cacheProto.CacheDeleteRequest{Key: shortCode})

	c.JSON(http.StatusOK, gin.H{"success": resp.Success})
}

func (h *Handler) recordClick(shortCode string, c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	h.analyticsClient.RecordClick(ctx, &analyticsProto.ClickEvent{
		ShortCode: shortCode,
		IpAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		Referer:   c.Request.Referer(),
		Country:   "",
	})
}

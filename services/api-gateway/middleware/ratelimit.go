package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"url-shortener/pkg/ratelimit"
)

type RateLimiter struct {
	limiter *ratelimit.Limiter
}

func NewRateLimiter(rate int, per time.Duration) *RateLimiter {
	return &RateLimiter{
		limiter: ratelimit.New(rate, per),
	}
}

func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		if !rl.limiter.Allow(ip) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

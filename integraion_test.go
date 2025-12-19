package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiGatewayURL = "http://localhost:8080"
	dbURL         = "postgres://urlshortener:password@localhost:5432/urlshortener?sslmode=disable"
)

type ShortenRequest struct {
	URL           string `json:"url"`
	UserID        string `json:"user_id,omitempty"`
	ExpiresInDays int64  `json:"expires_in_days,omitempty"`
}

type ShortenResponse struct {
	ShortCode string `json:"short_code"`
	ShortURL  string `json:"short_url"`
}

type StatsResponse struct {
	ShortCode      string `json:"short_code"`
	TotalClicks    int64  `json:"total_clicks"`
	UniqueVisitors int64  `json:"unique_visitors"`
	LastClickedAt  string `json:"last_clicked_at"`
}

// TestBasicFlow проверяет базовый сценарий создания и редиректа
func TestBasicFlow(t *testing.T) {
	// 1. Создание короткой ссылки
	reqBody := ShortenRequest{
		URL: "https://example.com/very/long/url/that/needs/shortening",
	}

	resp, err := createShortURL(reqBody)
	require.NoError(t, err, "Failed to create short URL")
	require.NotEmpty(t, resp.ShortCode, "Short code should not be empty")
	assert.Len(t, resp.ShortCode, 7, "Short code should be 7 characters")

	t.Logf("Created short code: %s", resp.ShortCode)

	// 2. Проверка редиректа
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Не следовать редиректу
		},
	}

	redirectResp, err := client.Get(fmt.Sprintf("%s/%s", apiGatewayURL, resp.ShortCode))
	require.NoError(t, err, "Failed to get redirect")
	defer redirectResp.Body.Close()

	assert.Equal(t, http.StatusMovedPermanently, redirectResp.StatusCode, "Should return 301")

	location := redirectResp.Header.Get("Location")
	assert.Equal(t, reqBody.URL, location, "Should redirect to original URL")

	t.Logf("Redirect test passed: %s -> %s", resp.ShortCode, location)
}

// TestDatabaseSchema проверяет схему базы данных
func TestDatabaseSchema(t *testing.T) {
	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db.Close()

	// Проверка таблицы urls
	var urlsExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_name = 'urls'
		)
	`).Scan(&urlsExists)
	require.NoError(t, err)
	assert.True(t, urlsExists, "Table 'urls' should exist")

	// Проверка таблицы clicks
	var clicksExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_name = 'clicks'
		)
	`).Scan(&clicksExists)
	require.NoError(t, err)
	assert.True(t, clicksExists, "Table 'clicks' should exist")

	// Проверка таблицы url_stats
	var statsExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_name = 'url_stats'
		)
	`).Scan(&statsExists)
	require.NoError(t, err)
	assert.True(t, statsExists, "Table 'url_stats' should exist")

	// Проверка индексов на urls
	var urlsIndexCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pg_indexes 
		WHERE tablename = 'urls'
	`).Scan(&urlsIndexCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, urlsIndexCount, 2, "Should have at least 2 indexes on urls")

	// Проверка индексов на clicks
	var clicksIndexCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pg_indexes 
		WHERE tablename = 'clicks'
	`).Scan(&clicksIndexCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, clicksIndexCount, 3, "Should have at least 3 indexes on clicks")

	t.Log("Database schema validation passed")
}

// TestGracefulShutdown проверяет graceful shutdown (требует ручного теста)
func TestGracefulShutdown(t *testing.T) {
	t.Skip("Graceful shutdown requires manual testing")

	// Этот тест должен проверять что при SIGTERM/SIGINT:
	// 1. Сервисы перестают принимать новые запросы
	// 2. Существующие запросы завершаются
	// 3. Worker pool flush'ит оставшиеся батчи
	// 4. Соединения с БД/Redis корректно закрываются
}

// Helper functions

func createShortURL(req ShortenRequest) (*ShortenResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/shorten", apiGatewayURL),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result ShortenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// TestMain выполняет подготовку перед тестами
func TestMain(m *testing.M) {
	// Проверка что сервисы запущены
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("Waiting for services to be ready...")

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Services are not ready after 30 seconds")
			return
		default:
			resp, err := http.Get(fmt.Sprintf("%s/stats/test", apiGatewayURL))
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusInternalServerError {
					fmt.Println("Services are ready!")
					m.Run()
					return
				}
			}
			time.Sleep(1 * time.Second)
		}
	}
}

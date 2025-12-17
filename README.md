# URL Shortener v2 - Краткое ТЗ (PostgreSQL + sqlc + Chi + Redis + gRPC + Analytics + Concurrency)

## Описание проекта

Сервис сокращения ссылок с системой аналитики. **Конкурентность критична**: обработка кликов должна быть асинхронной (не блокировать редирект), статистика пишется батчами для производительности.

## Базовые требования

### Стек

- **PostgreSQL 14+** + sqlc v1.27+ + golang-migrate
- **Chi router v5** для HTTP
- **Redis** для счетчиков и кэша (обязательно)
- **Многопоточность**: worker pools для analytics
- **Опционально**: gRPC + отдельный Analytics Service


### Архитектура

```
User → BFF/API → URL Service → PostgreSQL
                      ↓            ↑
                  Redis Cache   Analytics Worker Pool
                      ↓            ↓
                  Click Queue → Batch Insert
```


## Структура проекта

```
project/
├── url-service/
│   ├── main.go
│   ├── sqlc.yaml
│   ├── db/migrations/
│   ├── db/queries/
│   └── internal/
│       ├── db/              # sqlc generated
│       ├── worker/          # analytics workers
│       ├── handler/         # chi handlers
│       ├── shortener/       # hash generation
│       └── cache/           # redis layer
├── analytics-service/ (опц.)
│   └── internal/grpc/
└── proto/ (опц.)
```


## База данных

### Миграция 000001_create_urls.up.sql

```sql
CREATE TABLE urls (
    id SERIAL PRIMARY KEY,
    short_code VARCHAR(10) UNIQUE NOT NULL,
    original_url TEXT NOT NULL,
    user_id INT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE clicks (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) NOT NULL,
    clicked_at TIMESTAMPTZ DEFAULT NOW(),
    ip_address INET,
    user_agent TEXT,
    referer TEXT,
    country VARCHAR(2)
);

CREATE INDEX idx_urls_short_code ON urls(short_code);
CREATE INDEX idx_clicks_short_code ON clicks(short_code);
CREATE INDEX idx_clicks_clicked_at ON clicks(clicked_at);
```


### SQL запросы (db/queries/urls.sql)

```sql
-- name: CreateURL :one
INSERT INTO urls (short_code, original_url, expires_at)
VALUES ($1, $2, $3) RETURNING *;

-- name: GetURLByShortCode :one
SELECT * FROM urls WHERE short_code = $1 AND (expires_at IS NULL OR expires_at > NOW());

-- name: BatchInsertClicks :exec
INSERT INTO clicks (short_code, clicked_at, ip_address, user_agent, referer)
SELECT * FROM unnest($1::varchar[], $2::timestamptz[], $3::inet[], $4::text[], $5::text[]);

-- name: GetClickStats :one
SELECT 
    COUNT(*) as total_clicks,
    COUNT(DISTINCT ip_address) as unique_visitors
FROM clicks 
WHERE short_code = $1 
    AND clicked_at >= $2;
```


## Основные endpoints

### URL Service (Chi)

```
POST   /api/shorten          # Создать короткую ссылку
GET    /{shortCode}           # Редирект (асинхронная запись клика)
GET    /api/stats/{shortCode} # Статистика
DELETE /api/urls/{shortCode}  # Удалить
```


## Многопоточность (обязательно)

### 1. Асинхронная обработка кликов

**Критично**: редирект не должен ждать записи в БД.

```go
type ClickEvent struct {
    ShortCode string
    Timestamp time.Time
    IP        string
    UserAgent string
    Referer   string
}

var clickQueue = make(chan ClickEvent, 10000)

// Handler редиректа (быстрый)
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
    code := chi.URLParam(r, "code")
    
    // 1. Получить URL из Redis/DB
    url := h.cache.GetURL(code) // < 1ms
    
    // 2. Асинхронная отправка клика (не блокирует)
    select {
    case clickQueue <- ClickEvent{
        ShortCode: code,
        Timestamp: time.Now(),
        IP:        r.RemoteAddr,
        UserAgent: r.UserAgent(),
        Referer:   r.Referer(),
    }:
    default:
        // Очередь заполнена, логируем
    }
    
    // 3. Быстрый редирект
    http.Redirect(w, r, url, http.StatusMovedPermanently)
}
```


### 2. Worker Pool для батчевой записи

```go
type AnalyticsWorker struct {
    queries    *db.Queries
    clickQueue <-chan ClickEvent
    batchSize  int
    flushTime  time.Duration
}

func (w *AnalyticsWorker) Start(ctx context.Context) {
    batch := make([]ClickEvent, 0, w.batchSize)
    ticker := time.NewTicker(w.flushTime)
    
    for {
        select {
        case click := <-w.clickQueue:
            batch = append(batch, click)
            
            // Если батч заполнен, записываем
            if len(batch) >= w.batchSize {
                w.flushBatch(ctx, batch)
                batch = batch[:0]
            }
            
        case <-ticker.C:
            // Периодическая запись (даже если батч не полный)
            if len(batch) > 0 {
                w.flushBatch(ctx, batch)
                batch = batch[:0]
            }
            
        case <-ctx.Done():
            w.flushBatch(ctx, batch)
            return
        }
    }
}

func (w *AnalyticsWorker) flushBatch(ctx context.Context, batch []ClickEvent) {
    // Batch INSERT через sqlc
    shortCodes := make([]string, len(batch))
    timestamps := make([]time.Time, len(batch))
    ips := make([]string, len(batch))
    // ... остальные поля
    
    for i, click := range batch {
        shortCodes[i] = click.ShortCode
        timestamps[i] = click.Timestamp
        ips[i] = click.IP
    }
    
    w.queries.BatchInsertClicks(ctx, shortCodes, timestamps, ips, ...)
}
```


### 3. Redis для счетчиков (real-time)

```go
// Инкремент счетчика в Redis (быстро)
func (c *Cache) IncrementClicks(ctx context.Context, shortCode string) {
    key := "clicks:" + shortCode
    c.rdb.Incr(ctx, key)
    c.rdb.Expire(ctx, key, 24*time.Hour)
}

// Получить статистику (Redis + PostgreSQL)
func (s *Service) GetStats(ctx context.Context, code string) (*Stats, error) {
    var wg sync.WaitGroup
    var redisClicks int64
    var dbStats db.ClickStats
    
    // Параллельные запросы
    wg.Add(2)
    
    go func() {
        defer wg.Done()
        redisClicks = s.cache.GetClickCount(ctx, code)
    }()
    
    go func() {
        defer wg.Done()
        dbStats = s.queries.GetClickStats(ctx, code, time.Now().Add(-30*24*time.Hour))
    }()
    
    wg.Wait()
    
    return &Stats{
        TotalClicks: dbStats.TotalClicks + redisClicks,
        UniqueVisitors: dbStats.UniqueVisitors,
    }, nil
}
```


## Redis (обязательно)

### Кэширование URL

```go
// Cache-aside pattern
func (c *Cache) GetURL(shortCode string) (string, error) {
    key := "url:" + shortCode
    
    // 1. Попытка из кэша
    url, err := c.rdb.Get(ctx, key).Result()
    if err == nil {
        return url, nil
    }
    
    // 2. Из БД
    urlRecord, err := c.queries.GetURLByShortCode(ctx, shortCode)
    if err != nil {
        return "", err
    }
    
    // 3. Сохранить в кэш
    c.rdb.Set(ctx, key, urlRecord.OriginalURL, 1*time.Hour)
    
    return urlRecord.OriginalURL, nil
}
```


### Rate Limiting

```go
func (m *RateLimitMiddleware) Limit(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        key := "ratelimit:" + r.RemoteAddr
        
        count, _ := m.rdb.Incr(r.Context(), key).Result()
        if count == 1 {
            m.rdb.Expire(r.Context(), key, 1*time.Minute)
        }
        
        if count > 100 {
            http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
            return
        }
        
        next.ServeHTTP(w, r)
    })
}
```


## gRPC (опционально)

### proto/analytics.proto

```protobuf
syntax = "proto3";
package analytics;

service AnalyticsService {
  rpc RecordClick(ClickRequest) returns (ClickResponse);
  rpc GetStats(StatsRequest) returns (StatsResponse);
}

message ClickRequest {
  string short_code = 1;
  string ip_address = 2;
  string user_agent = 3;
}

message StatsResponse {
  int64 total_clicks = 1;
  int64 unique_visitors = 2;
  repeated DailyStats daily_breakdown = 3;
}
```


## Тестирование

### 1. Race detector

```bash
go test -race ./...
```


### 2. Benchmark тесты

```go
func BenchmarkRedirectWithBatch(b *testing.B) {
    // С батчингом
    worker := NewAnalyticsWorker(queries, clickQueue, 100, 5*time.Second)
    worker.Start(context.Background())
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            handleRedirect(...)
        }
    })
}

func BenchmarkRedirectNoBatch(b *testing.B) {
    // Без батчинга (прямая запись в DB)
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            handleRedirect(...)
            db.InsertClick(...) // Блокирует
        }
    })
}
```

**Ожидаемый результат**: батчинг в 10-50x быстрее.

### 3. Load test

```bash
# Создать 1000 коротких ссылок
ab -n 1000 -c 10 -p create.json http://localhost:8080/api/shorten

# Симулировать клики (должно быть быстро)
ab -n 10000 -c 100 http://localhost:8080/abc123
```

**Проверка**: 10000 редиректов за < 5 секунд (200-300ms без батчинга).

### 4. Проверка батчинга

```go
func TestBatchInsertion(t *testing.T) {
    // Отправить 150 кликов
    for i := 0; i < 150; i++ {
        clickQueue <- ClickEvent{...}
    }
    
    time.Sleep(6 * time.Second) // Ждем flush
    
    // Проверить что был 1 batch INSERT, а не 150 отдельных
    clicks := db.GetClicks(...)
    assert.Equal(t, 150, len(clicks))
    
    // Проверить логи: должно быть "Flushed batch of 100" + "Flushed batch of 50"
}
```


### 5. Мониторинг горутин

```go
r.Get("/debug/metrics", func(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(map[string]interface{}{
        "goroutines": runtime.NumGoroutine(),
        "queue_size": len(clickQueue),
        "redis_hits": cacheHits,
    })
})
```

**Ожидание**: 5-20 горутин (workers + chi), queue_size растет под нагрузкой.

## Критерии оценки

### Минимум (v1)

- ✅ POST /api/shorten создает короткую ссылку
- ✅ GET /{code} редиректит
- ✅ PostgreSQL + sqlc работают
- ✅ **Redis кэширует URLs**
- ✅ **Клики записываются (хотя бы синхронно)**


### Хорошо (v2)

- ✅ + **Worker pool для кликов с батчингом**
- ✅ + **Редирект не блокируется записью клика**
- ✅ + **Redis счетчики для real-time статистики**
- ✅ + **Rate limiting через Redis**
- ✅ + **go test -race проходит**
- ✅ + Graceful shutdown с flush оставшихся батчей


### Отлично (v2+)

- ✅ + **gRPC Analytics Service**
- ✅ + **Benchmark показывает 10x улучшение**
- ✅ + **Load test: 10k req/s на редирект**
- ✅ + **Мониторинг: /debug/metrics endpoint**
- ✅ + Custom short codes (vanity URLs)
- ✅ + QR code generation
- ✅ + Expiration URLs
- ✅ + Geo-analytics (IP → country)


## Простая проверка конкурентности

```bash
# 1. Запустить сервис
make run

# 2. Открыть метрики
curl http://localhost:8080/debug/metrics

# 3. Создать нагрузку
ab -n 10000 -c 100 http://localhost:8080/test123

# 4. Снова проверить метрики
curl http://localhost:8080/debug/metrics
```

**Если работает правильно**:

- `queue_size` растет до 100-1000 под нагрузкой
- `goroutines` = 5-20 (стабильно)
- Редирект занимает < 5ms
- В логах: "Flushed batch of 100 clicks"

**Если НЕ работает**:

- `queue_size` = 0 (нет очереди)
- `goroutines` = 1-2
- Редирект > 50ms
- В логах: 10000 отдельных INSERT запросов
# url-shortener

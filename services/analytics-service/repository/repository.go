package repository

import (
	"database/sql"
	"time"
	"url-shortener/services/analytics-service/models"

	sq "github.com/Masterminds/squirrel"
)

type Repository struct {
	db *sql.DB
	qb sq.StatementBuilderType
}

func New(db *sql.DB) *Repository {
	return &Repository{
		db: db,
		qb: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

type Stats struct {
	TotalClicks    int64
	UniqueVisitors int64
	LastClickedAt  *time.Time
}

func (r *Repository) BatchInsertClicks(events []models.ClickEvent) error {
	if len(events) == 0 {
		return nil
	}

	query := r.qb.Insert("clicks").
		Columns("short_code", "clicked_at", "ip_address", "user_agent", "referer", "country")

	for _, event := range events {
		query = query.Values(
			event.ShortCode,
			event.Timestamp,
			event.IPAddress,
			event.UserAgent,
			event.Referer,
			event.Country,
		)
	}

	_, err := query.RunWith(r.db).Exec()
	return err
}

func (r *Repository) UpdateStats(shortCode string) error {
	query := `
		INSERT INTO url_stats (short_code, total_clicks, unique_visitors, last_clicked_at, updated_at)
		VALUES ($1, 1, 1, NOW(), NOW())
		ON CONFLICT (short_code) 
		DO UPDATE SET 
			total_clicks = url_stats.total_clicks + 1,
			last_clicked_at = NOW(),
			updated_at = NOW()
	`
	_, err := r.db.Exec(query, shortCode)
	return err
}

func (r *Repository) GetStats(shortCode string) (*Stats, error) {
	query := r.qb.Select("total_clicks", "unique_visitors", "last_clicked_at").
		From("url_stats").
		Where(sq.Eq{"short_code": shortCode})

	var stats Stats
	err := query.RunWith(r.db).QueryRow().Scan(
		&stats.TotalClicks,
		&stats.UniqueVisitors,
		&stats.LastClickedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return &Stats{}, nil
		}
		return nil, err
	}
	return &stats, nil
}

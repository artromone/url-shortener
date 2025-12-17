package repository

import (
	"database/sql"
	"time"

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

type URL struct {
	ID          int64
	ShortCode   string
	OriginalURL string
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	UserID      string
	IsActive    bool
}

func (r *Repository) Create(url *URL) error {
	query := r.qb.Insert("urls").
		Columns("short_code", "original_url", "user_id", "expires_at", "is_active").
		Values(url.ShortCode, url.OriginalURL, url.UserID, url.ExpiresAt, url.IsActive).
		Suffix("RETURNING id, created_at")

	return query.RunWith(r.db).QueryRow().Scan(&url.ID, &url.CreatedAt)
}

func (r *Repository) GetByShortCode(shortCode string) (*URL, error) {
	query := r.qb.Select("id", "short_code", "original_url", "created_at", "expires_at", "user_id", "is_active").
		From("urls").
		Where(sq.Eq{"short_code": shortCode})

	var url URL
	err := query.RunWith(r.db).QueryRow().Scan(
		&url.ID, &url.ShortCode, &url.OriginalURL, &url.CreatedAt,
		&url.ExpiresAt, &url.UserID, &url.IsActive,
	)
	if err != nil {
		return nil, err
	}
	return &url, nil
}

func (r *Repository) Delete(shortCode string) error {
	query := r.qb.Update("urls").
		Set("is_active", false).
		Where(sq.Eq{"short_code": shortCode})

	_, err := query.RunWith(r.db).Exec()
	return err
}

func (r *Repository) Exists(shortCode string) (bool, error) {
	query := r.qb.Select("COUNT(*)").
		From("urls").
		Where(sq.Eq{"short_code": shortCode})

	var count int
	err := query.RunWith(r.db).QueryRow().Scan(&count)
	return count > 0, err
}

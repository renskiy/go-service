package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"go-service/internal/service/models"
)

type Repository struct {
	db *sqlx.DB
}

func New(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

const addScoreSQL = `
insert into scores (id, score, updated_at)
values (:id, :score, :updated_at)
on conflict (id) do update set
    score = score + :score,
    updated_at = :now
returning id, score, updated_at
`

func (r *Repository) AddScore(ctx context.Context, id int64, score float64) (*models.Score, error) {
	result := new(models.Score)
	query, args, err := r.db.BindNamed(addScoreSQL, map[string]interface{}{
		"id":         id,
		"score":      score,
		"updated_at": time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("could not bind addScoreSQL: %w", err)
	}
	if err = r.db.GetContext(ctx, result, query, args...); err != nil {
		return nil, fmt.Errorf("could not execute addScoreSQL: %w", err)
	}
	return result, nil
}

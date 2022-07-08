package models

import (
	"time"
)

type Score struct {
	ID        int64     `db:"id"`
	Score     float64   `db:"score"`
	UpdatedAt time.Time `db:"updated_at"`
	Neighbors []int64   `db:"neighbors"`
}

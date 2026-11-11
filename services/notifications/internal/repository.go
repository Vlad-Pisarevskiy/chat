package internal

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {

	return &Repository{pool: pool}
}

func (r *Repository) SaveNotify(senderID, receiverID, chatID int) error {
	return errors.New("notification persistence is not implemented")
}

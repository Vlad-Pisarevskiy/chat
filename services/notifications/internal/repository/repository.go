package repository

import (
	"chatflow/services/notifications/internal/model"
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {

	return &Repository{pool: pool}
}

func (r *Repository) SaveNotify(ctx context.Context, notification model.Notification) error {

	_, err := r.pool.Exec(ctx, `INSERT INTO notifications(chat_id, sender_id, recipients_ids, title, preview, date) 
						  VALUES($1, $2, $3, $4, $5)`,
		notification.ChatID, notification.SenderID, notification.RecipientsIDs,
		notification.Title, notification.Preview, time.Now())

	if err != nil {
		return err
	}

	return nil
}

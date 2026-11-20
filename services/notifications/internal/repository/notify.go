package repository

import (
	"chatflow/services/notifications/internal/model"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type NotifyRepository struct {
	tx DBTX
}

func NewRepository(tx DBTX) *NotifyRepository {

	return &NotifyRepository{tx: tx}
}

func (r *NotifyRepository) SaveNotify(ctx context.Context, notification model.Notification) (*model.NotifyFromDB, error) {

	row := r.tx.QueryRow(ctx, `INSERT INTO notifications(chat_id, sender_id, recipients_ids, title, preview, date) 
						  VALUES($1, $2, $3, $4, $5)`,
		notification.ChatID, notification.SenderID, notification.RecipientsIDs,
		notification.Title, notification.Preview, time.Now())

	var notify *model.NotifyFromDB
	if err := row.Scan(&notify); err != nil {
		return nil, err
	}

	return notify, nil
}

func (r *NotifyRepository) ChangeStatus(ctx context.Context, id int, status string) error {

	_, err := r.tx.Exec(ctx, `UPDATE notifications
								    SET status = $1 WHERE id = $2`, status, id)

	if err != nil {
		return err
	}

	return nil

}

func (r *NotifyRepository) AddMissingID(ctx context.Context, userID int) error {

	_, err := r.tx.Exec(ctx, `INSERT INTO missing (id) VALUES ($1)`, userID)
	if err != nil {
		return err
	}

	return nil
}

func (r *NotifyRepository) RemoveMissingID(ctx context.Context, userID int) error {

	_, err := r.tx.Exec(ctx, `DELETE FROM missing WHERE id = $1`, userID)
	if err != nil {
		return err
	}

	return nil
}

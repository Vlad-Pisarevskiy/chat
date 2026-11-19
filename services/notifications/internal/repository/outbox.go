package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type OutboxRepository struct {
	tx DBTX
}
type OutboxTask struct {
	Type          string
	Status        string
	ChatID        int
	SenderID      int
	RecipientsIDs []int
	Title         string
	Preview       string
}

type TaskFromDB struct {
	ID            int    `db:"id"`
	Type          string `db:"type"`
	Status        string `db:"status"`
	ChatID        int    `db:"chat_id"`
	SenderID      int    `db:"sender_id"`
	RecipientsIDs []int  `db:"recipients_ids"`
	Title         string `db:"title"`
	Preview       string `db:"preview"`
}

func NewOutboxRepo(tx DBTX) *OutboxRepository {
	return &OutboxRepository{tx: tx}
}

func (r *OutboxRepository) AddTask(ctx context.Context, task OutboxTask) error {

	_, err := r.tx.Exec(ctx, `INSERT INTO outbox (type, done, chat_id, sender_id, recipients_ids, title, preview)
									VALUES($1,$2,$3,$4,$5,$6,$7)`,
		task.Type,
		task.Status,
		task.ChatID,
		task.SenderID,
		task.RecipientsIDs,
		task.Title,
		task.Preview)

	if err != nil {
		return err
	}

	return nil
}

func (r *OutboxRepository) GetTasks(ctx context.Context) ([]TaskFromDB, error) {

	rows, err := r.tx.Query(ctx, `SELECT FROM outbox WHERE status = 'pending' LIMIT 50`)
	if err != nil {
		return nil, err
	}

	tasks, err := pgx.CollectRows(rows, pgx.RowToStructByName[TaskFromDB])
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

func (r *OutboxRepository) ClaimFailed(ctx context.Context, taskID int) error {

	_, err := r.tx.Exec(ctx, `UPDATE outbox SET status = $1 WHERE task_id = $2`,
		"failed", taskID)

	if err != nil {
		return err
	}
}

func (r *OutboxRepository) ChangeStatus(ctx context.Context, id int, status string) error {

	_, err := r.tx.Exec(ctx, `UPDATE notifications
								    SET status = $1 WHERE id = $2`, status, id)

	if err != nil {
		return err
	}

	return nil

}

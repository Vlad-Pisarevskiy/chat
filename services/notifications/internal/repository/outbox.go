package repository

import (
	"context"
)

type OutboxRepository struct {
	tx DBTX
}
type OutboxTask struct {
	Type          string
	Done          bool
	ChatID        int
	SenderID      int
	RecipientsIDs []int
	Title         string
	Preview       string
}

func NewOutboxRepo(tx DBTX) *OutboxRepository {
	return &OutboxRepository{tx: tx}
}

func (r *OutboxRepository) AddTask(ctx context.Context, task OutboxTask) error {

	_, err := r.tx.Exec(ctx, `INSERT INTO outbox (type, done, chat_id, sender_id, recipients_ids, title, preview)
									VALUES($1,$2,$3,$4,$5,$6,$7)`,
		task.Type,
		task.Done,
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

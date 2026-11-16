package main

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	db *pgxpool.Pool
}

type TaskFromDB struct {
	Type          string `db:"type"`
	Status        string `db:"status"`
	ChatID        int    `db:"chat_id"`
	SenderID      int    `db:"sender_id"`
	RecipientsIDs []int  `db:"recipients_ids"`
	Title         string `db:"title"`
	Preview       string `db:"preview"`
}

func NewWorker(db *pgxpool.Pool) *Worker {

	return &Worker{db: db}
}

func (w *Worker) Run(ctx context.Context) error {

	for {

		rows, err := w.db.Query(ctx, `SELECT FROM outbox WHERE status = 'pending' LIMIT 50`)
		if err != nil {
			return err
		}

		tasks, err := pgx.CollectRows(rows, pgx.RowToStructByName[TaskFromDB])
		if err != nil {
			return err
		}
	}
}

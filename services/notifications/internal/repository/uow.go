package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repos struct {
	Notify NotifyRepository
	Outbox OutboxRepository
}

type UnitOfWork struct {
	db *pgxpool.Pool
}

func NewUnitOfWork(db *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{db: db}
}

func (u *UnitOfWork) Do(ctx context.Context, fn func(r Repos) error) error {

	tx, err := u.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err = fn(Repos{
		Notify: NotifyRepository{tx},
		Outbox: OutboxRepository{tx},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
